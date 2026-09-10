package spk

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote/file"
)

var errStorageMisbehaved = errors.New("discard_test: open kernel: Access is denied")

// seedKernel writes a byte into a fresh bucket and returns it with its key, so
// a test can ask whether the file survived a decision.
func seedKernel(t *testing.T) (*file.Bucket, string) {
	t.Helper()

	bucket, err := file.Open(t.Context(), testutil.FileURL(t, t.TempDir()))
	if err != nil {
		t.Fatalf("open bucket: %v", err)
	}

	const key = "jpl/planets/de440s.bsp"

	// file.Save rather than bucket.WriteAll: these tests are t.Parallel and
	// each has its own t.TempDir bucket, but fileblob stages every write in
	// os.TempDir under a name built from the key's basename and a Windows
	// clock that does not move — so separate buckets writing "de440s.bsp"
	// still collide. Save holds the staging lock that prevents it (#241).
	if err := file.Save(t.Context(), bucket, key, strings.NewReader("kernel bytes")); err != nil {
		t.Fatalf("seed kernel: %v", err)
	}

	return bucket, key
}

func exists(t *testing.T, bucket *file.Bucket, key string) bool {
	t.Helper()

	ok, err := bucket.Exists(t.Context(), key)
	if err != nil {
		t.Fatalf("exists %s: %v", key, err)
	}

	return ok
}

// TestDiscardIfCorruptKeepsAKernelItMerelyCouldNotRead is the whole point of
// the helper.
//
// A cached kernel that cannot be read and one whose bytes are wrong are
// different facts wanting opposite responses, and they used to share a branch.
// The wrong half of that is destructive and shared: `go test ./...` runs
// packages as separate processes against one cache directory, so a package
// losing a race to a file lock deleted a 32 MB kernel out from under the
// others, which then spent three minutes each failing to find it. It is the
// observed cause of intermittent Windows CI failures on main (#227).
func TestDiscardIfCorruptKeepsAKernelItMerelyCouldNotRead(t *testing.T) {
	t.Parallel()

	bucket, key := seedKernel(t)

	closed := false
	err := discardIfCorrupt(t.Context(), bucket, key,
		func() error { closed = true; return nil },
		errStorageMisbehaved)

	if !errors.Is(err, errStorageMisbehaved) {
		t.Errorf("err = %v, want it to wrap the underlying failure", err)
	}

	if !closed {
		t.Error("the file handle was not closed; it is closed on both paths")
	}

	if !exists(t, bucket, key) {
		t.Error("the kernel was deleted after a read failure that says nothing " +
			"about its content; the next open should have found it and tried again")
	}
}

// TestDiscardIfCorruptDeletesAKernelThatIsWrong is the other half: the
// auto-heal this helper must not lose while it stops over-reaching.
//
// A checksum that computed cleanly and did not match is a statement about the
// bytes, and the cached copy is worthless. Deleting it is what makes the next
// run heal itself, which is the behaviour CacheDownload's comment has always
// claimed.
func TestDiscardIfCorruptDeletesAKernelThatIsWrong(t *testing.T) {
	t.Parallel()

	bucket, key := seedKernel(t)

	const sidecar = "jpl/planets/de440s.bsp.sha256"

	if err := bucket.WriteAll(t.Context(), sidecar, []byte("deadbeef"), nil); err != nil {
		t.Fatalf("seed sidecar: %v", err)
	}

	cause := errors.New("sha256 mismatch") //nolint:err113 // a stand-in for the real message, wrapped below

	closed := false
	err := discardIfCorrupt(t.Context(), bucket, key,
		func() error { closed = true; return nil },
		errors.Join(ErrCorruptSPK, cause),
		func() error { return bucket.Delete(t.Context(), sidecar) })

	if !errors.Is(err, ErrCorruptSPK) {
		t.Errorf("err = %v, want it to keep reporting ErrCorruptSPK", err)
	}

	if !closed {
		t.Error("the file handle was not closed")
	}

	if exists(t, bucket, key) {
		t.Error("a kernel whose checksum did not match was left in the cache; " +
			"the next run would keep failing on the same bytes")
	}

	// The sidecar describes the kernel and must not outlive it, or the next
	// download's bootstrap compares against a recording of the corrupt one.
	if exists(t, bucket, sidecar) {
		t.Error("the checksum sidecar outlived the kernel it describes")
	}
}

// TestDiscardIfCorruptRunsExtrasOnlyWhenItDeletes pins the asymmetry.
//
// The extra cleanups exist to remove things that describe the kernel. Running
// them on a read failure would delete the checksum sidecar of a kernel that is
// still there and still valid, silently turning the next open's verification
// into a fresh bootstrap — which would then record whatever the file contains,
// including corruption it was supposed to catch.
func TestDiscardIfCorruptRunsExtrasOnlyWhenItDeletes(t *testing.T) {
	t.Parallel()

	bucket, key := seedKernel(t)

	ran := 0
	extra := func() error { ran++; return nil }

	_ = discardIfCorrupt(t.Context(), bucket, key, func() error { return nil },
		errStorageMisbehaved, extra)

	if ran != 0 {
		t.Errorf("extra cleanup ran %d times after a read failure, want 0", ran)
	}

	_ = discardIfCorrupt(t.Context(), bucket, key, func() error { return nil },
		ErrCorruptSPK, extra)

	if ran != 1 {
		t.Errorf("extra cleanup ran %d times after a corruption finding, want 1", ran)
	}
}

// TestDiscardIfCorruptReportsACloseFailure covers the handle, which is closed
// on both paths and whose failure must not be swallowed by either.
func TestDiscardIfCorruptReportsACloseFailure(t *testing.T) {
	t.Parallel()

	closeFailed := errors.New("discard_test: close failed") //nolint:err113 // a stand-in, joined below

	bucket, key := seedKernel(t)

	for _, tc := range []struct {
		name  string
		cause error
	}{
		{"after a read failure", errStorageMisbehaved},
		{"after a corruption finding", ErrCorruptSPK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := discardIfCorrupt(t.Context(), bucket, key,
				func() error { return closeFailed }, tc.cause)

			if !errors.Is(err, closeFailed) {
				t.Errorf("err = %v, want it to carry the close failure too", err)
			}

			if !errors.Is(err, tc.cause) {
				t.Errorf("err = %v, want it to still carry the cause", err)
			}
		})
	}
}

// TestNewReaderCallsAShortFileCorrupt keeps the auto-heal working for the one
// read failure that really is a statement about the content.
//
// A file too short to hold an SPK's own 1024-byte file record cannot be a
// valid kernel however well the storage behaved, so it carries ErrCorruptSPK
// and a cached copy is discarded. Without this, splitting I/O failures from
// content failures would have quietly stopped truncated downloads from healing.
func TestNewReaderCallsAShortFileCorrupt(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		size int
	}{
		{"empty", 0},
		{"one byte", 1},
		{"one byte short of a file record", RecordSize - 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewReader(nopCloser{io.NewSectionReader(
				readerAtFunc(make([]byte, tc.size)), 0, int64(tc.size))})

			if !errors.Is(err, ErrCorruptSPK) {
				t.Errorf("err = %v, want ErrCorruptSPK — a file shorter than its own "+
					"file record is bad content, not a storage failure", err)
			}
		})
	}
}

// TestNewReaderKeepsARealReadFailureAsOne is the complement: a storage error
// that is not a short read must not be dressed up as corruption, or it deletes
// a kernel that is fine.
func TestNewReaderKeepsARealReadFailureAsOne(t *testing.T) {
	t.Parallel()

	_, err := NewReader(nopCloser{failingReaderAt{}})

	if errors.Is(err, ErrCorruptSPK) {
		t.Errorf("err = %v, want it not to claim corruption for a storage failure", err)
	}

	if !errors.Is(err, errStorageMisbehaved) {
		t.Errorf("err = %v, want it to wrap the storage failure", err)
	}
}

// readerAtFunc serves a fixed byte slice, returning io.EOF past its end the
// way any short file does.
type readerAtFunc []byte

func (b readerAtFunc) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(b)) {
		return 0, io.EOF
	}

	n := copy(p, b[off:])
	if n < len(p) {
		return n, io.EOF
	}

	return n, nil
}

// failingReaderAt is storage that is broken rather than short.
type failingReaderAt struct{}

func (failingReaderAt) ReadAt([]byte, int64) (int, error) { return 0, errStorageMisbehaved }

type nopCloser struct{ io.ReaderAt }

func (nopCloser) Close() error { return nil }
