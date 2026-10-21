package file

import (
	"bytes"
	"errors"
	"io/fs"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/time"
)

// The chunk cache under concurrent readers (#811): a resident chunk is read
// without waiting on anybody's miss, a cold chunk is read once however many
// readers want it, residency stays bounded, a failed read is not kept, and
// Close waits for the reads in flight.

const (
	hookChunk    = 16
	hookResident = 4
)

// hookFile is an in-memory object whose ReadAt calls are counted per offset
// and can be held or failed by a hook.
type hookFile struct {
	*bytes.Reader

	fsys *hookFS
}

func (f *hookFile) Stat() (fs.FileInfo, error) { return hookInfo{size: f.Size()}, nil }

func (f *hookFile) ReadAt(p []byte, off int64) (int, error) {
	f.fsys.mu.Lock()
	f.fsys.reads[off]++
	hook := f.fsys.hook
	f.fsys.mu.Unlock()

	if hook != nil {
		if err := hook(off); err != nil {
			return 0, err
		}
	}

	return f.Reader.ReadAt(p, off) //nolint:wrapcheck // a pass-through
}

func (f *hookFile) Close() error {
	f.fsys.closed.Store(true)

	return nil
}

type hookFS struct {
	data   []byte
	mu     sync.Mutex
	reads  map[int64]int
	hook   func(off int64) error
	closed atomic.Bool
}

func newHookFS(data []byte) *hookFS { return &hookFS{data: data, reads: map[int64]int{}} }

func (h *hookFS) Open(string) (fs.File, error) {
	return &hookFile{Reader: bytes.NewReader(h.data), fsys: h}, nil
}

func (h *hookFS) readsAt(off int64) int {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.reads[off]
}

type hookInfo struct {
	fs.FileInfo

	size int64
}

func (i hookInfo) Size() int64 { return i.size }

// openHooked is a ReaderAt over fsys with hookChunk-byte chunks, hookResident
// resident.
func openHooked(t *testing.T, fsys *hookFS) *ReaderAt {
	t.Helper()

	f, err := Open(t.Context(), fsys, "obj", WithChunkSize(hookChunk), WithCachedChunks(hookResident))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	ra, _ := f.(*ReaderAt)

	return ra
}

func readChunk(t *testing.T, ra *ReaderAt, data []byte, i int) {
	t.Helper()

	buf := make([]byte, hookChunk)
	if _, err := ra.ReadAt(buf, int64(i*hookChunk)); err != nil {
		t.Errorf("chunk %d: %v", i, err)

		return
	}

	if !bytes.Equal(buf, data[i*hookChunk:(i+1)*hookChunk]) {
		t.Errorf("chunk %d: wrong bytes", i)
	}
}

// within fails the test if done does not close within limit — long enough
// that a correct implementation never comes near it, short enough that a
// deadlock fails rather than hangs the package.
func within(t *testing.T, done <-chan struct{}, what string) {
	t.Helper()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("%s did not finish", what)
	}
}

// heldRead holds every read of off until release is called, signaling when
// one has started.
func heldRead(fsys *hookFS, off int64) (started <-chan struct{}, release func()) {
	s := make(chan struct{})
	r := make(chan struct{})

	var once sync.Once

	fsys.hook = func(o int64) error {
		if o == off {
			once.Do(func() { close(s) })
			<-r
		}

		return nil
	}

	return s, func() { close(r) }
}

// TestReaderAtHitDoesNotWaitForAMiss is #811: a read of a resident chunk
// completes while another reader's miss is held inside the backend. The cache
// used to hold its one mutex across that backend read, so the hit waited for
// it — a pread, or over HTTP a network round trip.
func TestReaderAtHitDoesNotWaitForAMiss(t *testing.T) {
	data := payload(4 * hookChunk)
	fsys := newHookFS(data)
	ra := openHooked(t, fsys)

	readChunk(t, ra, data, 0) // resident from here on

	started, release := heldRead(fsys, 1*hookChunk)

	missed := make(chan struct{})

	go func() {
		defer close(missed)

		readChunk(t, ra, data, 1)
	}()

	<-started

	hit := make(chan struct{})

	go func() {
		defer close(hit)

		readChunk(t, ra, data, 0)
	}()

	within(t, hit, "a read of a resident chunk, while another chunk's read was held")

	release()
	within(t, missed, "the held read, once released")
}

// TestReaderAtReadsAColdChunkOnce: sixteen readers of one cold chunk, the
// first of them held inside the backend while the rest arrive, cost one
// backend read; the others wait for it rather than each issuing their own.
func TestReaderAtReadsAColdChunkOnce(t *testing.T) {
	data := payload(4 * hookChunk)
	fsys := newHookFS(data)
	ra := openHooked(t, fsys)

	started, release := heldRead(fsys, 2*hookChunk)

	var wg sync.WaitGroup

	for range 16 {
		wg.Go(func() { readChunk(t, ra, data, 2) })
	}

	<-started
	release()

	done := make(chan struct{})

	go func() {
		wg.Wait()
		close(done)
	}()

	within(t, done, "sixteen readers of one chunk")

	if n := fsys.readsAt(2 * hookChunk); n != 1 {
		t.Errorf("the chunk was read from the backend %d times, want 1", n)
	}
}

// TestReaderAtStaysWithinItsResidentBound: however many chunks are read, no
// more than the resident bound stay cached; and a chunk read again since it
// arrived survives the eviction that follows, which is what CLOCK's second
// chance is for.
func TestReaderAtStaysWithinItsResidentBound(t *testing.T) {
	const bound = hookResident

	data := payload(64 * hookChunk)
	fsys := newHookFS(data)
	ra := openHooked(t, fsys)

	resident := func() int {
		n := 0

		ra.chunks.Range(func(any, any) bool { n++; return true })

		return n
	}

	for i := range 64 {
		readChunk(t, ra, data, i)

		if n := resident(); n > bound {
			t.Fatalf("after chunk %d: %d chunks resident, bound %d", i, n, bound)
		}
	}

	ra2 := openHooked(t, newHookFS(data))
	for i := range bound {
		readChunk(t, ra2, data, i)
	}

	readChunk(t, ra2, data, 0) // read again: referenced
	readChunk(t, ra2, data, bound)

	if _, ok := ra2.chunks.Load(int64(0)); !ok {
		t.Error("chunk 0, read again since it arrived, was evicted ahead of chunks read once")
	}
}

var errFlaky = errors.New("a backend read that fails once")

// TestReaderAtDoesNotKeepAFailedRead: a chunk whose read failed is not
// cached as failed; the next reader reads it again.
func TestReaderAtDoesNotKeepAFailedRead(t *testing.T) {
	data := payload(4 * hookChunk)
	fsys := newHookFS(data)
	ra := openHooked(t, fsys)

	var failed atomic.Bool

	fsys.hook = func(off int64) error {
		if off == hookChunk && !failed.Swap(true) {
			return errFlaky
		}

		return nil
	}

	if _, err := ra.ReadAt(make([]byte, hookChunk), hookChunk); !errors.Is(err, errFlaky) {
		t.Fatalf("first read: err = %v, want the backend's", err)
	}

	readChunk(t, ra, data, 1)

	if n := fsys.readsAt(hookChunk); n != 2 {
		t.Errorf("the chunk was read %d times, want 2: the failure, then the retry", n)
	}
}

// TestCloseWaitsForReadsInFlight: Close does not close the backend file under
// a read still in progress on it, and ReadAt fails once Close has returned.
func TestCloseWaitsForReadsInFlight(t *testing.T) {
	data := payload(4 * hookChunk)
	fsys := newHookFS(data)
	ra := openHooked(t, fsys)

	started, release := heldRead(fsys, hookChunk)

	read := make(chan struct{})

	go func() {
		defer close(read)

		readChunk(t, ra, data, 1)
	}()

	<-started

	closed := make(chan struct{})

	go func() {
		defer close(closed)

		if err := ra.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	select {
	case <-closed:
		t.Fatal("Close returned while a read was still in flight")
	case <-time.After(50 * time.Millisecond):
	}

	if fsys.closed.Load() {
		t.Fatal("the backend file was closed under a read in flight")
	}

	release()
	within(t, read, "the held read")
	within(t, closed, "Close, once the read finished")

	if _, err := ra.ReadAt(make([]byte, 4), 0); !errors.Is(err, ErrReaderAtClosed) {
		t.Errorf("ReadAt after Close: err = %v, want ErrReaderAtClosed", err)
	}
}
