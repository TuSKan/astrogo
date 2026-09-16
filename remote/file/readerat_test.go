package file

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/time"
)

// payload is deterministic and longer than several chunks at the sizes the
// tests use, so a wrong chunk offset shows up as wrong bytes rather than a
// short read.
func payload(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte('a' + i%26)
	}

	return b
}

func seedFS(t *testing.T, key string, data []byte) fs.FS {
	t.Helper()

	fsys, err := OpenFS(mustLocalURL(t, t.TempDir()))
	if err != nil {
		t.Fatalf("OpenFS: %v", err)
	}

	if err := WriteFile(t.Context(), fsys, key, bytes.NewReader(data)); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	return fsys
}

func TestReaderAtMatchesBytesReaderAcrossChunkBoundaries(t *testing.T) {
	const chunk = 64

	data := payload(chunk*4 + 7) // deliberately not a whole number of chunks
	fsys := seedFS(t, "obj", data)

	ra, err := Open(t.Context(), fsys, "obj", WithChunkSize(chunk), WithCachedChunks(2))
	if err != nil {
		t.Fatalf("NewReaderAt: %v", err)
	}
	defer func() { _ = ra.Close() }()

	if got := sizeOf(t, ra); got != int64(len(data)) {
		t.Fatalf("Stat().Size() = %d, want %d", got, len(data))
	}

	want := bytes.NewReader(data)

	// Every (offset, length) pair that straddles, aligns with, or falls
	// inside a chunk, compared against the standard library's own
	// io.ReaderAt so both the bytes and the (n, err) pair must agree.
	for off := 0; off <= len(data); off++ {
		for _, size := range []int{1, chunk - 1, chunk, chunk + 1, chunk * 3} {
			gotBuf, wantBuf := make([]byte, size), make([]byte, size)

			gotN, gotErr := ra.ReadAt(gotBuf, int64(off))
			wantN, wantErr := want.ReadAt(wantBuf, int64(off))

			if gotN != wantN || !errors.Is(gotErr, wantErr) {
				t.Fatalf("ReadAt(len=%d, off=%d) = (%d, %v), want (%d, %v)", size, off, gotN, gotErr, wantN, wantErr)
			}

			if !bytes.Equal(gotBuf[:gotN], wantBuf[:wantN]) {
				t.Fatalf("ReadAt(len=%d, off=%d) bytes mismatch", size, off)
			}
		}
	}
}

func TestReaderAtEdgeCases(t *testing.T) {
	data := payload(100)
	fsys := seedFS(t, "obj", data)

	ra, err := Open(t.Context(), fsys, "obj", WithChunkSize(32))
	if err != nil {
		t.Fatalf("NewReaderAt: %v", err)
	}

	if n, err := ra.ReadAt(nil, 0); n != 0 || err != nil {
		t.Errorf("ReadAt(empty) = (%d, %v), want (0, nil)", n, err)
	}

	if n, err := ra.ReadAt(make([]byte, 4), int64(len(data))); n != 0 || !errors.Is(err, io.EOF) {
		t.Errorf("ReadAt at EOF = (%d, %v), want (0, io.EOF)", n, err)
	}

	if _, err := ra.ReadAt(make([]byte, 4), -1); err == nil {
		t.Error("expected an error for a negative offset")
	}

	if err := ra.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := ra.ReadAt(make([]byte, 4), 0); !errors.Is(err, ErrReaderAtClosed) {
		t.Errorf("ReadAt after Close = %v, want ErrReaderAtClosed", err)
	}
}

func TestReaderAtConcurrentReads(t *testing.T) {
	const chunk = 16

	data := payload(chunk * 8)
	fsys := seedFS(t, "obj", data)

	// Two resident chunks against eight, so readers race on eviction too,
	// not just on a warm cache.
	ra, err := Open(t.Context(), fsys, "obj", WithChunkSize(chunk), WithCachedChunks(2))
	if err != nil {
		t.Fatalf("NewReaderAt: %v", err)
	}
	defer func() { _ = ra.Close() }()

	var wg sync.WaitGroup

	for i := range 8 {
		off := i

		wg.Go(func() {
			for range 50 {
				buf := make([]byte, chunk)
				if _, err := ra.ReadAt(buf, int64(off*chunk)); err != nil {
					t.Errorf("ReadAt(off=%d): %v", off*chunk, err)

					return
				}

				if !bytes.Equal(buf, data[off*chunk:(off+1)*chunk]) {
					t.Errorf("ReadAt(off=%d) returned wrong bytes", off*chunk)

					return
				}
			}
		})
	}

	wg.Wait()
}

// The point of this package's design: random access works over a source
// that has no OS path at all. httpblob is registered by default, so an
// http:// fsys needs no opt-in import.
func TestReaderAtOverHTTPBucket(t *testing.T) {
	data := payload(1000)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "obj", time.GoTime{}, bytes.NewReader(data))
	}))
	defer srv.Close()

	fsys, err := OpenFS(srv.URL + "/")
	if err != nil {
		t.Fatalf("OpenFS http: %v", err)
	}

	ra, err := Open(t.Context(), fsys, "obj", WithChunkSize(128))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = ra.Close() }()

	if got := sizeOf(t, ra); got != int64(len(data)) {
		t.Fatalf("Stat().Size() = %d, want %d", got, len(data))
	}

	got := make([]byte, 300)
	if _, err := ra.ReadAt(got, 500); err != nil {
		t.Fatalf("ReadAt: %v", err)
	}

	if !bytes.Equal(got, data[500:800]) {
		t.Error("ReadAt over an http:// fsys returned wrong bytes")
	}
}

func TestWriteOverHTTPIsRefusedAsReadOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "read-only", http.StatusMethodNotAllowed)
	}))
	defer srv.Close()

	fsys, err := OpenFS(srv.URL + "/")
	if err != nil {
		t.Fatalf("OpenFS http: %v", err)
	}

	// The refusal is now structural rather than a server's answer: the HTTP
	// backend does not implement CreateFS, so the write never leaves the
	// process. That is a better failure than a 405, and it is the same one on
	// every server including those that would have accepted a PUT.
	err = WriteFile(t.Context(), fsys, "k", strings.NewReader("x"))
	if !errors.Is(err, ErrReadOnly) {
		t.Errorf("writing to an http:// filesystem returned %v, want ErrReadOnly", err)
	}
}

// sizeOf reads an open file's size through Stat, which is where fs.File puts
// it. The concrete *ReaderAt also has Size(), but the interface callers hold is
// File and Stat is what they have.
func sizeOf(t *testing.T, f File) int64 {
	t.Helper()

	info, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	return info.Size()
}
