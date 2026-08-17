package file

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
)

// Defaults for NewReaderAt: 64 KiB chunks, 16 resident, so a reader holds
// at most 1 MiB no matter how large the object is — a 3 GB kernel and a
// 5 KB one cost the same.
//
// Measured against the access pattern that motivates this type (SPK
// segment evaluation: thousands of ~100-byte reads clustered in a few
// regions), over a 64 MB object on local fileblob, 2000 reads:
//
//	one range read per ReadAt   256 ms
//	4 KiB chunks                6.9 ms
//	64 KiB chunks               0.32 ms   <- default
//	1 MiB chunks                0.37 ms   (16x the memory, no faster)
//
// See BenchmarkReadAtStrategies.
const (
	defaultChunkSize    = 64 << 10
	defaultCachedChunks = 16
)

// readerAtConfig carries NewReaderAt's options.
type readerAtConfig struct {
	chunkSize    int64
	cachedChunks int
}

// ReaderAtOption customizes a NewReaderAt call.
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

// ReaderAt is a random-access view of one object, satisfying io.ReaderAt
// and io.Closer. It reads whole aligned chunks through the bucket's range
// reader and keeps the most recently used ones, so a burst of small reads
// within a region costs one transfer rather than one per call.
//
// Memory is bounded by chunk size times resident chunks — 1 MiB by
// default — and never scales with the object. Chunks are evicted
// least-recently-used; the object is not buffered.
//
// This matters on every backend, not just remote ones: fileblob opens a
// fresh OS file handle on each NewRangeReader, so an uncached ReadAt is
// nowhere near the cost of a pread, and over HTTP or S3 it is one request
// per read. Chunking fixes that generically, with no per-driver
// reach-through and no local-only fast path.
//
// Safe for concurrent use.
type ReaderAt struct {
	bucket *Bucket
	key    string
	size   int64

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

// NewReaderAt opens bucket/key for random access. The object's size is
// read once here; a ReaderAt therefore observes the object as it was at
// this moment and is not meant to outlive a rewrite of that key.
func NewReaderAt(ctx context.Context, bucket *Bucket, key string, opts ...ReaderAtOption) (*ReaderAt, error) {
	cfg := readerAtConfig{chunkSize: defaultChunkSize, cachedChunks: defaultCachedChunks}
	for _, opt := range opts {
		opt(&cfg)
	}

	attrs, err := bucket.Attributes(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("remote/file: attributes %s: %w", key, err)
	}

	return &ReaderAt{
		bucket:    bucket,
		key:       key,
		size:      attrs.Size,
		chunkSize: cfg.chunkSize,
		maxChunks: cfg.cachedChunks,
		chunks:    make(map[int64][]byte, cfg.cachedChunks),
		lru:       list.New(),
		elems:     make(map[int64]*list.Element, cfg.cachedChunks),
	}, nil
}

// Size reports the object's size in bytes as observed at open time.
func (r *ReaderAt) Size() int64 { return r.size }

// ReadAt implements io.ReaderAt: it fills p entirely or returns a non-nil
// error, and reports io.EOF when it stops short at the end of the object.
func (r *ReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, fmt.Errorf("%w: read %s at %d", ErrNegativeOffset, r.key, off)
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

// Close releases the cached chunks. The underlying Bucket is shared and
// process-lived, so it is deliberately left open.
func (r *ReaderAt) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.closed = true
	r.chunks = nil
	r.elems = nil
	r.lru = list.New()

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

	// Background context, not a caller-supplied one: io.ReaderAt has no
	// ctx parameter, and a ReaderAt is handed to parsers (spk.Reader) that
	// hold it well past the call that opened it.
	rc, err := r.bucket.NewRangeReader(context.Background(), r.key, offset, length, nil)
	if err != nil {
		return nil, fmt.Errorf("remote/file: range read %s at %d: %w", r.key, offset, err)
	}

	buf := make([]byte, length)

	_, err = io.ReadFull(rc, buf)
	closeErr := rc.Close()

	if err != nil || closeErr != nil {
		return nil, fmt.Errorf("remote/file: range read %s at %d: %w", r.key, offset, errors.Join(err, closeErr))
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
