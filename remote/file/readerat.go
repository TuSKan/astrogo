package file

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sync"
)

// Defaults for [Open]: 64 KiB chunks, 16 resident, so a reader holds at most
// 1 MiB no matter how large the object is — a 3 GB kernel and a 5 KB one cost
// the same.
//
// Measured against the access pattern that motivates this type (SPK segment
// evaluation: thousands of ~100-byte reads clustered in a few regions), over a
// 64 MiB object, 2000 reads. The first column is the backend's own ReadAt with
// no cache in front of it; see BenchmarkReadAtStrategies and the table in
// fsys_bench_test.go.
//
//	                  uncached      4 KiB    64 KiB     1 MiB
//	file://             4.5 ms     6.9 ms   0.053 ms   0.056 ms
//	http             11,400 ms          —          —          —
//
// 64 KiB is the default because 1 MiB costs sixteen times the memory and is not
// faster. The http row has one entry because the uncached figure — 2000 ranged
// GETs against a server on localhost — settles the question without needing the
// others.
const (
	defaultChunkSize    = 64 << 10
	defaultCachedChunks = 16
)

// readerAtConfig carries [Open]'s options.
type readerAtConfig struct {
	chunkSize    int64
	cachedChunks int
}

// ReaderAtOption customizes an [Open] call.
type ReaderAtOption func(*readerAtConfig)

// WithChunkSize sets the read granularity in bytes. A reader holds at most
// this times WithCachedChunks. Values below 1 are ignored.
func WithChunkSize(n int64) ReaderAtOption {
	return func(c *readerAtConfig) {
		if n > 0 {
			c.chunkSize = n
		}
	}
}

// WithCachedChunks sets how many chunks stay resident, bounding memory at
// that times WithChunkSize. Values below 1 are ignored.
func WithCachedChunks(n int) ReaderAtOption {
	return func(c *readerAtConfig) {
		if n > 0 {
			c.cachedChunks = n
		}
	}
}

// ReaderAt is a chunk-caching view of one open object. It is what [Open]
// returns, and it implements [File], so a caller sees an ordinary open file.
//
// # Why the cache is inside Open rather than beside it
//
// Every backend here already implements io.ReaderAt, so a caller could use one
// directly and the type would be unnecessary. Measured, it is not: over HTTP an
// uncached ReadAt is one ranged GET per call, which for SPK-shaped access is
// 2000 requests and eleven seconds against a server on localhost. Locally it is
// a pread — 48x cheaper than the gocloud path it replaces, and still 84x slower
// than serving the same bytes from a resident chunk.
//
// So the cache is not an optimisation a caller opts into and forgets; it is the
// difference between the layer working and not. Making Open the only door means
// no call site can get it wrong, which is the same reason remote is the only
// door to this package.
//
// Reads of whole aligned chunks are cached and evicted least-recently-used.
// Memory is bounded by chunk size times resident chunks and never scales with
// the object; the object is not buffered. Sequential Read and Seek pass through
// to the underlying file untouched, so a whole-object download stays one
// request rather than being reassembled from chunks.
//
// Safe for concurrent use.
type ReaderAt struct {
	src  File
	name string
	size int64

	chunkSize int64
	maxChunks int

	mu     sync.Mutex
	closed bool
	chunks map[int64][]byte
	lru    *list.List              // front = most recently used; values are int64 chunk indexes
	elems  map[int64]*list.Element // chunk index -> its lru element
}

// Sentinel errors returned by ReaderAt. Match with errors.Is.
var (
	// ErrReaderAtClosed is returned by ReadAt after Close.
	ErrReaderAtClosed = errors.New("remote/file: ReaderAt is closed")

	// ErrNegativeOffset is returned by ReadAt for an offset below zero,
	// which io.ReaderAt leaves to the implementation to reject.
	ErrNegativeOffset = errors.New("remote/file: negative offset")
)

// Open opens name on fsys for reading, with the chunk cache described on
// [ReaderAt] in front of it.
//
// The object's size is read once here, so a File observes the object as it was
// at this moment and is not meant to outlive a rewrite of that name.
//
// A filesystem implementing [ContextFS] is bound to ctx first, so a read that
// blocks on somebody else's network can be canceled. One that does not — the
// local backend, which has nothing to cancel — is used as it is.
func Open(ctx context.Context, fsys fs.FS, name string, opts ...ReaderAtOption) (File, error) {
	cfg := readerAtConfig{chunkSize: defaultChunkSize, cachedChunks: defaultCachedChunks}
	for _, opt := range opts {
		opt(&cfg)
	}

	f, err := WithContext(ctx, fsys).Open(name)
	if err != nil {
		return nil, fmt.Errorf("remote/file: open %s: %w", name, err)
	}

	src, ok := f.(File)
	if !ok {
		_ = f.Close()

		return nil, fmt.Errorf("remote/file: open %s: %w", name, ErrNotSeekable)
	}

	info, err := f.Stat()
	if err != nil {
		_ = f.Close()

		return nil, fmt.Errorf("remote/file: stat %s: %w", name, err)
	}

	return &ReaderAt{
		src:       src,
		name:      name,
		size:      info.Size(),
		chunkSize: cfg.chunkSize,
		maxChunks: cfg.cachedChunks,
		chunks:    make(map[int64][]byte, cfg.cachedChunks),
		lru:       list.New(),
		elems:     make(map[int64]*list.Element, cfg.cachedChunks),
	}, nil
}

// Size reports the object's size in bytes as observed at open time.
func (r *ReaderAt) Size() int64 { return r.size }

// Stat implements [fs.File].
func (r *ReaderAt) Stat() (fs.FileInfo, error) {
	return r.src.Stat() //nolint:wrapcheck // the backend's own *fs.PathError is the better message
}

// Read implements [io.Reader], passing straight through to the underlying file.
//
// Deliberately not served from the chunk cache. The cache exists for scattered
// small reads; a sequential read is the download path, where the HTTP backend
// holds one body open across calls and reassembling it from ranged chunks would
// turn one request into hundreds.
func (r *ReaderAt) Read(p []byte) (int, error) {
	return r.src.Read(p) //nolint:wrapcheck // io.EOF must reach the caller unwrapped
}

// Seek implements [io.Seeker], passing through for the same reason Read does.
func (r *ReaderAt) Seek(offset int64, whence int) (int64, error) {
	return r.src.Seek(offset, whence) //nolint:wrapcheck // already a *fs.PathError
}

// ReadAt implements io.ReaderAt: it fills p entirely or returns a non-nil
// error, and reports io.EOF when it stops short at the end of the object.
func (r *ReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, fmt.Errorf("%w: read %s at %d", ErrNegativeOffset, r.name, off)
	}

	if len(p) == 0 {
		return 0, nil
	}

	if off >= r.size {
		return 0, io.EOF
	}

	var n int

	for n < len(p) {
		if off+int64(n) >= r.size {
			return n, io.EOF
		}

		idx := (off + int64(n)) / r.chunkSize

		chunk, err := r.chunk(idx)
		if err != nil {
			return n, err
		}

		within := (off + int64(n)) - idx*r.chunkSize
		n += copy(p[n:], chunk[within:])
	}

	return n, nil
}

// Close releases the cached chunks and closes the underlying file.
//
// Unlike the gocloud implementation this replaces, the underlying handle is
// this reader's own rather than a process-lived shared Bucket, so closing it
// here is both correct and necessary — an unclosed one is a leaked descriptor
// or a held HTTP connection.
func (r *ReaderAt) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.closed = true
	r.chunks = nil
	r.elems = nil
	r.lru = list.New()

	if err := r.src.Close(); err != nil {
		return fmt.Errorf("remote/file: close %s: %w", r.name, err)
	}

	return nil
}

// chunk returns chunk idx, fetching and caching it on a miss.
func (r *ReaderAt) chunk(idx int64) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil, ErrReaderAtClosed
	}

	if c, ok := r.chunks[idx]; ok {
		r.lru.MoveToFront(r.elems[idx])

		return c, nil
	}

	offset := idx * r.chunkSize

	length := r.chunkSize
	if rest := r.size - offset; rest < length {
		length = rest
	}

	buf := make([]byte, length)

	// The underlying ReadAt, not Read: io.ReaderAt is contractually safe for
	// concurrent use and moves no cursor, which is what lets this type be safe
	// for concurrent use while Read and Seek pass through to the same file.
	if _, err := r.src.ReadAt(buf, offset); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("remote/file: read %s at %d: %w", r.name, offset, err)
	}

	r.chunks[idx] = buf
	r.elems[idx] = r.lru.PushFront(idx)

	for r.lru.Len() > r.maxChunks {
		oldest := r.lru.Back()
		evict, _ := oldest.Value.(int64)

		r.lru.Remove(oldest)
		delete(r.chunks, evict)
		delete(r.elems, evict)
	}

	return buf, nil
}
