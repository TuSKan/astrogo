package file_test

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote/file"
)

// TestConcurrentSaveOfOneKey is the regression test for #241, where writing one
// cache key from several goroutines failed about one time in six on Windows.
//
// # What was wrong
//
// fileblob stages every write through a temporary file whose name it builds
// from time.Now().UnixNano(), on the stated assumption that the clock advances
// between attempts. On Windows it ticks about every 15.6 ms, and measured on
// Windows 11 it returned one distinct value across 2000 consecutive reads. So
// concurrent writers of one key all chose the same staging name, the O_EXCL
// retry loop re-read the same frozen clock and retried into the same name, and
// one writer's file was renamed out from under another.
//
// It surfaced as three unrelated-looking flaky tests in ephemeris/jpl/spk —
// each passing alone, a different one failing on each parallel run — and, in
// production code, as partial reads of a kernel another goroutine was still
// writing. The second is the one that mattered: a truncated kernel read is a
// wrong answer rather than an error, until a checksum catches it.
//
// # Why the test writes real bytes rather than a token
//
// A payload large enough that the write cannot complete inside one clock tick
// is what makes the window real. With a few bytes every writer finishes before
// the next scheduling point and the collision is much rarer, so a test built on
// a short string would have passed against the defect.
func TestConcurrentSaveOfOneKey(t *testing.T) {
	t.Parallel()

	const (
		writers = 8
		rounds  = 8
		key     = "jpl/planets/de440s.bsp"
	)

	payload := bytes.Repeat([]byte("kernel bytes "), 40_000) // ~520 KB

	ctx := context.Background()

	for round := range rounds {
		bucket, err := file.Open(ctx, testutil.FileURL(t, t.TempDir()))
		if err != nil {
			t.Fatalf("round %d: opening the bucket: %v", round, err)
		}

		errs := make([]error, writers)

		var wg sync.WaitGroup

		for i := range writers {
			wg.Go(func() {
				errs[i] = file.Save(ctx, bucket, key, bytes.NewReader(payload))
			})
		}

		wg.Wait()

		for i, err := range errs {
			if err != nil {
				t.Errorf("round %d, writer %d: %v.\n"+
					"  Concurrent writes of one cache key must all succeed. A rename error "+
					"naming a .tmp file is fileblob's clock-derived staging name colliding "+
					"(#241); the guard for it is file.WriteLock.", round, i, err)
			}
		}

		// And the object has to be the whole payload, not a torn prefix of it.
		// A collision does not always fail: it can also commit whatever bytes
		// had arrived, which is the failure mode that reads as data.
		got, err := bucket.ReadAll(ctx, key)
		if err != nil {
			t.Fatalf("round %d: reading back: %v", round, err)
		}

		if !bytes.Equal(got, payload) {
			t.Fatalf("round %d: read back %d bytes, wrote %d — the object is torn",
				round, len(got), len(payload))
		}
	}
}

// TestConcurrentSaveOfDistinctKeys covers the other half of #241: buckets that
// share nothing but a name.
//
// Before the fix these collided too, and that is the more surprising half.
// fileblob's staging file went into os.TempDir named after the *basename* of
// the key, so two buckets in different directories writing "…/de440s.bsp"
// contended over one file in a third place entirely. Measured, eight writers
// into eight separate buckets: 59 failures in 320.
//
// no_tmp_dir=1 on the bucket URL is what closes it, by putting staging inside
// the bucket. testutil.FileURL sets it, which is why this passes; the writers
// here hold different buckets, so file.WriteLock — keyed by bucket as well as
// key — deliberately does not serialise them and cannot be what saves it.
func TestConcurrentSaveOfDistinctKeys(t *testing.T) {
	t.Parallel()

	const (
		writers = 8
		rounds  = 8
		key     = "jpl/planets/de440s.bsp"
	)

	payload := bytes.Repeat([]byte("kernel bytes "), 40_000)

	ctx := context.Background()

	for round := range rounds {
		buckets := make([]*file.Bucket, writers)

		for i := range writers {
			b, err := file.Open(ctx, testutil.FileURL(t, t.TempDir()))
			if err != nil {
				t.Fatalf("round %d: opening bucket %d: %v", round, i, err)
			}

			buckets[i] = b
		}

		errs := make([]error, writers)

		var wg sync.WaitGroup

		for i := range writers {
			wg.Go(func() {
				errs[i] = file.Save(ctx, buckets[i], key, bytes.NewReader(payload))
			})
		}

		wg.Wait()

		for i, err := range errs {
			if err != nil {
				t.Errorf("round %d, writer %d: %v.\n"+
					"  Separate buckets writing the same key must not interfere. If the error "+
					"names a path under the system temp directory, the bucket URL has lost "+
					"no_tmp_dir=1 (#241).", round, i, err)
			}
		}
	}
}

// TestWriteLockIsExclusive pins the guard itself rather than its effect.
//
// The concurrency tests above can pass for the wrong reason — a scheduler that
// happens not to overlap the writers, or a clock that happens to advance — so
// one of them asserts the property directly: while a lock is held, no second
// holder of the same key exists, and a different key is not blocked by it.
func TestWriteLockIsExclusive(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	bucket, err := file.Open(ctx, testutil.FileURL(t, t.TempDir()))
	if err != nil {
		t.Fatalf("opening the bucket: %v", err)
	}

	t.Run("one key excludes itself", func(t *testing.T) {
		t.Parallel()

		var (
			mu      sync.Mutex
			holders int
			worst   int
			wg      sync.WaitGroup
		)

		for range 8 {
			wg.Go(func() {
				for range 200 {
					unlock := file.WriteLock(bucket, "same/key")

					mu.Lock()
					holders++

					if holders > worst {
						worst = holders
					}

					mu.Unlock()

					mu.Lock()
					holders--
					mu.Unlock()

					unlock()
				}
			})
		}

		wg.Wait()

		if worst != 1 {
			t.Errorf("saw %d simultaneous holders of one key, want 1", worst)
		}
	})

	t.Run("a different key is not blocked", func(t *testing.T) {
		t.Parallel()

		// Held for the duration, so a lock on another key that waited for it
		// would hang the test rather than fail it — which is the honest
		// outcome for a deadlock and is what a timeout reports.
		release := file.WriteLock(bucket, "held/key")
		defer release()

		done := make(chan struct{})

		go func() {
			defer close(done)

			file.WriteLock(bucket, "other/key")()
		}()

		<-done
	})

	t.Run("the same key in another bucket is not blocked", func(t *testing.T) {
		t.Parallel()

		other, err := file.Open(ctx, testutil.FileURL(t, t.TempDir()))
		if err != nil {
			t.Fatalf("opening the second bucket: %v", err)
		}

		release := file.WriteLock(bucket, "shared/name")
		defer release()

		done := make(chan struct{})

		go func() {
			defer close(done)

			file.WriteLock(other, "shared/name")()
		}()

		<-done
	})
}

// TestFileURLCarriesTheStagingParameters guards the two query parameters the
// concurrency above depends on, in the one place tests get a bucket URL.
//
// They are easy to drop and nothing else notices: a URL without create_dir
// fails loudly on a first run, but a URL without no_tmp_dir keeps working and
// merely goes back to colliding one time in six, on Windows, under
// parallelism — which is how this arrived as three unrelated flaky tests.
func TestFileURLCarriesTheStagingParameters(t *testing.T) {
	t.Parallel()

	got := testutil.FileURL(t, t.TempDir())

	for _, want := range []string{"create_dir=true", "no_tmp_dir=1"} {
		if !strings.Contains(got, want) {
			t.Errorf("testutil.FileURL produced %q, which is missing %q (#241)", got, want)
		}
	}
}
