package file

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"sync"
	"time"
)

// httpFile is one object, read by range requests.
//
// # Read and ReadAt do different things on purpose
//
// ReadAt is required by [io.ReaderAt] to be safe for concurrent use and to leave
// no cursor behind, so each call is its own ranged GET. That is what makes
// remote's chunked reader — sixteen 64 KiB chunks, 1 MiB resident for any
// object — able to serve a three-gigabyte kernel from a few requests.
//
// Read is sequential and keeps one response body open across calls, because
// issuing a request per Read would be a request per buffer-full. Seek discards
// that body; the next Read opens a new one at the new offset. A caller that
// only ever reads forward therefore makes exactly one request.
type httpFile struct {
	fsys *httpFS
	url  string
	info httpInfo

	mu     sync.Mutex
	offset int64
	body   io.ReadCloser
	// bodyAt is the offset body was opened at, so a Read can tell whether the
	// open body is still the right one.
	bodyAt int64
}

// Stat implements [fs.File].
func (f *httpFile) Stat() (fs.FileInfo, error) { return f.info, nil }

// Close implements [fs.File].
func (f *httpFile) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.closeBody()
}

// Read implements [io.Reader].
func (f *httpFile) Read(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.offset >= f.info.size && f.info.size > 0 {
		return 0, io.EOF
	}

	if f.body == nil || f.bodyAt != f.offset {
		if err := f.closeBody(); err != nil {
			return 0, err
		}

		// The body is deliberately not closed here: holding it open across Read
		// calls is this type's whole reason for existing, and it is closed by
		// closeBody from Seek, from Close, and from the branch above when a
		// Read arrives at an offset this body is not positioned at.
		//
		//nolint:bodyclose // stored in f.body and closed by closeBody
		resp, err := f.fsys.do(f.fsys.context(), http.MethodGet, f.url,
			fmt.Sprintf("bytes=%d-", f.offset))
		if err != nil {
			return 0, err
		}

		f.body = resp.Body
		f.bodyAt = f.offset
	}

	n, err := f.body.Read(p)
	f.offset += int64(n)
	f.bodyAt = f.offset

	if err != nil && !errors.Is(err, io.EOF) {
		return n, fmt.Errorf("remote/file: read %s: %w", f.url, err)
	}

	return n, err //nolint:wrapcheck // io.EOF must reach the caller unwrapped
}

// ReadAt implements [io.ReaderAt].
//
// Per that interface's contract this is safe to call concurrently, does not
// move the Read cursor, and returns a non-nil error when it reads fewer than
// len(p) bytes — which is what lets a caller distinguish a short object from a
// truncated transfer.
func (f *httpFile) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, &fs.PathError{Op: "readat", Path: f.url, Err: fs.ErrInvalid}
	}

	if len(p) == 0 {
		return 0, nil
	}

	if f.info.size > 0 && off >= f.info.size {
		return 0, io.EOF
	}

	rng := fmt.Sprintf("bytes=%d-%d", off, off+int64(len(p))-1)

	resp, err := f.fsys.do(f.fsys.context(), http.MethodGet, f.url, rng)
	if err != nil {
		return 0, err
	}

	defer func() { _ = resp.Body.Close() }()

	n, err := io.ReadFull(resp.Body, p)
	if errors.Is(err, io.ErrUnexpectedEOF) {
		// Fewer bytes than asked for because the object ended, which ReaderAt
		// reports as io.EOF alongside the count.
		return n, io.EOF
	}

	if err != nil {
		return n, fmt.Errorf("remote/file: read %s at %d: %w", f.url, off, err)
	}

	return n, nil
}

// Seek implements [io.Seeker].
//
// Seeking is free: it moves a number and drops any open body. The cost lands on
// the next Read, which opens a new range.
func (f *httpFile) Seek(offset int64, whence int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var abs int64

	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = f.offset + offset
	case io.SeekEnd:
		abs = f.info.size + offset
	default:
		return 0, &fs.PathError{Op: "seek", Path: f.url, Err: fs.ErrInvalid}
	}

	if abs < 0 {
		return 0, &fs.PathError{Op: "seek", Path: f.url, Err: fs.ErrInvalid}
	}

	if abs != f.offset {
		if err := f.closeBody(); err != nil {
			return 0, err
		}
	}

	f.offset = abs

	return abs, nil
}

func (f *httpFile) closeBody() error {
	if f.body == nil {
		return nil
	}

	err := f.body.Close()
	f.body = nil

	if err != nil {
		return fmt.Errorf("remote/file: close %s: %w", f.url, err)
	}

	return nil
}

// httpInfo is an object's metadata.
//
// Mode is 0444 and IsDir is false, always: this filesystem serves objects, and
// an HTTP server's directory listing is HTML rather than something io/fs can
// describe. A caller wanting a listing wants remote's endpoint registry, which
// records the files an endpoint serves.
type httpInfo struct {
	name    string
	size    int64
	modTime time.Time
}

func (i httpInfo) Name() string       { return i.name }
func (i httpInfo) Size() int64        { return i.size }
func (i httpInfo) Mode() fs.FileMode  { return 0o444 }
func (i httpInfo) ModTime() time.Time { return i.modTime }
func (i httpInfo) IsDir() bool        { return false }
func (i httpInfo) Sys() any           { return nil }
