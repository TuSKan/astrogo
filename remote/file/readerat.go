package file

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"slices"
	"sync"
	"sync/atomic"
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
// So the cache is not an optimization a caller opts into and forgets; it is the
// difference between the layer working and not. Making Open the only door means
// no call site can get it wrong, which is the same reason remote is the only
// door to this package.
//
// Reads of whole aligned chunks are cached and evicted by CLOCK, the
// second-chance approximation of least-recently-used. Memory is bounded by
// chunk size times resident chunks and never scales with the object; the
// object is not buffered. Sequential Read and Seek pass through to the
// underlying file untouched, so a whole-object download stays one request
// rather than being reassembled from chunks.
//
// # Concurrency
//
// Safe for concurrent use, and built for it: a jpl.Provider is shared by every
// goroutine of a scheduler or a service, and all of them read its kernels
// through one of these. A read of a resident chunk takes no lock at all — a
// lock-free map lookup and, at most, setting the chunk's referenced bit. A
// read that misses claims the chunk under a short lock, then reads it with no
// lock held; any other reader wanting that chunk meanwhile waits for that one
// read rather than issuing its own.
//
// It used to hold one mutex across every lookup and across the backend read
// of every miss — a pread, or a ranged GET over HTTP — so sixteen goroutines
// evaluating the Moon at random epochs got less done in aggregate than one
// (#811). Measured on de440s, sixteen now take 481 ns per state in aggregate
// against 2,052 before, and against 457 for the same kernel held in a byte
// slice with no cache in front of it: the cache no longer costs concurrency
// anything. Eviction still takes the lock, which is cheap: it only runs when a
// chunk is claimed. Memory stays bounded at resident chunks, plus — only while
// every resident chunk is itself still being read — the chunks being read
// beyond them, each held by a ReadAt in flight.
type ReaderAt struct {
	src  File
	name string
	size int64

	chunkSize int64
	maxChunks int

	// chunks maps a chunk index to its *chunk. Reads look a chunk up without
	// any lock; mu is taken only to add one or evict one.
	chunks sync.Map

	mu     sync.Mutex
	closed bool
	ring   []int64        // the indexes of the chunks in chunks, in CLOCK order
	hand   int            // the CLOCK hand: the next index in ring to consider
	loads  sync.WaitGroup // chunk reads in flight, which Close waits for
}

// chunk is one aligned chunk of the object: being read until done, then
// holding the bytes or the error the read returned.
//
// A hit checks done, one atomic load; only a reader that arrives while the
// read is in flight waits on reading. That was a channel, received from on
// every hit, and a receive that blocks takes the channel's lock even once the
// channel is closed, which put every reader of a hot chunk back on one lock.
// A WaitGroup is also part of the chunk rather than an allocation beside it.
type chunk struct {
	reading sync.WaitGroup
	done    atomic.Bool
	data    []byte
	err     error
	ref     atomic.Bool // read since the CLOCK hand last passed it
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
		ring:      make([]int64, 0, cfg.cachedChunks),
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
	r.closed = true
	r.mu.Unlock()

	// No chunk can be claimed once closed is set, so the reads in flight are
	// all the reads there will be; they finish before the file they read from
	// is closed under them.
	r.loads.Wait()

	r.mu.Lock()
	r.chunks.Clear()
	r.ring, r.hand = nil, 0
	r.mu.Unlock()

	if err := r.src.Close(); err != nil {
		return fmt.Errorf("remote/file: close %s: %w", r.name, err)
	}

	return nil
}

// chunk returns chunk idx: resident, or being read by another reader and
// waited for, or claimed and read by this one.
func (r *ReaderAt) chunk(idx int64) ([]byte, error) {
	if v, ok := r.chunks.Load(idx); ok {
		c, _ := v.(*chunk)

		return c.wait()
	}

	c, mine, err := r.claim(idx)
	if err != nil {
		return nil, err
	}

	if !mine {
		return c.wait() // another reader claimed it between the lookup and the lock
	}

	return r.load(idx, c)
}

// wait returns the chunk's bytes once its read has finished, marking it
// referenced for CLOCK.
func (c *chunk) wait() ([]byte, error) {
	if !c.done.Load() {
		c.reading.Wait()
	}

	if c.err != nil {
		return nil, c.err
	}

	// Most reads find the bit set already; reading before writing keeps a hot
	// chunk's cache line shared across cores.
	if !c.ref.Load() {
		c.ref.Store(true)
	}

	return c.data, nil
}

// claim adds an unread chunk idx to the cache, evicting to make room, and
// returns it for the caller to read — or, when another reader has claimed it
// first, returns that reader's chunk with mine false.
func (r *ReaderAt) claim(idx int64) (c *chunk, mine bool, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.closed {
		return nil, false, ErrReaderAtClosed
	}

	if v, ok := r.chunks.Load(idx); ok {
		c, _ = v.(*chunk)

		return c, false, nil
	}

	r.evictLocked()

	c = new(chunk)
	c.reading.Add(1)
	r.chunks.Store(idx, c)

	// The new chunk goes just behind the hand, so it is the last one the hand
	// reaches: a full revolution before it can be evicted.
	r.ring = slices.Insert(r.ring, r.hand, idx)
	r.hand = (r.hand + 1) % len(r.ring)

	r.loads.Add(1)

	return c, true, nil
}

// evictLocked makes room for one chunk by CLOCK: the hand sweeps the ring,
// giving each referenced chunk a second chance by clearing its bit, and
// evicts the first unreferenced chunk that is no longer being read. A chunk
// being read is passed over; when every chunk is, nothing is evicted and the
// cache holds one more until a read finishes.
func (r *ReaderAt) evictLocked() {
	for range 2 * len(r.ring) {
		if len(r.ring) < r.maxChunks {
			return
		}

		k := r.ring[r.hand]
		v, _ := r.chunks.Load(k)
		c, _ := v.(*chunk)

		if !c.done.Load() {
			r.hand = (r.hand + 1) % len(r.ring)

			continue // still being read
		}

		if c.ref.Swap(false) {
			r.hand = (r.hand + 1) % len(r.ring)

			continue // a second chance
		}

		r.chunks.Delete(k)
		r.ring = slices.Delete(r.ring, r.hand, r.hand+1)

		if len(r.ring) > 0 {
			r.hand %= len(r.ring)
		} else {
			r.hand = 0
		}
	}
}

// load reads the claimed chunk c with no lock held and publishes the result
// to every reader waiting on it. A failed read is not kept: the next reader
// of that chunk reads it again.
func (r *ReaderAt) load(idx int64, c *chunk) ([]byte, error) {
	defer r.loads.Done()

	offset := idx * r.chunkSize
	length := min(r.chunkSize, r.size-offset)
	buf := make([]byte, length)

	// The underlying ReadAt, not Read: io.ReaderAt is contractually safe for
	// concurrent use and moves no cursor, which is what lets this type be safe
	// for concurrent use while Read and Seek pass through to the same file.
	if _, err := r.src.ReadAt(buf, offset); err != nil && !errors.Is(err, io.EOF) {
		c.err = fmt.Errorf("remote/file: read %s at %d: %w", r.name, offset, err)
		r.forget(idx, c)
	} else {
		c.data = buf
	}

	c.done.Store(true)
	c.reading.Done()

	return c.data, c.err
}

// forget removes chunk idx from the cache if c is still the chunk held there.
func (r *ReaderAt) forget(idx int64, c *chunk) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if v, ok := r.chunks.Load(idx); !ok || v != c {
		return
	}

	r.chunks.Delete(idx)

	if i := slices.Index(r.ring, idx); i >= 0 {
		r.ring = slices.Delete(r.ring, i, i+1)

		switch {
		case len(r.ring) == 0:
			r.hand = 0
		case i < r.hand:
			r.hand--
		default:
			r.hand %= len(r.ring)
		}
	}
}
