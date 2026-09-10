package file

import (
	"context"
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// TestWriteLockIsExclusive pins the guard itself rather than its effect.
//
// In-package, because writeLock is not exported. It is plumbing for one
// driver's naming bug rather than something a caller of an astronomy library
// has any use for, and [Save] is the door that holds it. Testing it directly
// still matters: the concurrency tests beside it exercise the lock through a
// write, so they can pass for the wrong reason.
//
// The concurrency tests above can pass for the wrong reason — a scheduler that
// happens not to overlap the writers, or a clock that happens to advance — so
// one of them asserts the property directly: while a lock is held, no second
// holder of the same staging path exists, that a genuinely different name is not
// blocked by it, and that the same name in another bucket IS.
func TestWriteLockIsExclusive(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	bucket, err := Open(ctx, testutil.FileURL(t, t.TempDir()))
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
					unlock := writeLock(bucket, "same/key")

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

	t.Run("a different name is not blocked", func(t *testing.T) {
		t.Parallel()

		// Different BASENAMES, which is the distinction that matters. This
		// used to say "held/key" and "other/key" — different keys, one
		// basename, and therefore one staging path. Under the current key they
		// deadlock, correctly, and the test that named them "different" was
		// describing a property the collision domain does not have.
		release := writeLock(bucket, "held/de440s.bsp")
		defer release()

		done := make(chan struct{})

		go func() {
			defer close(done)

			writeLock(bucket, "other/de441.bsp")()
		}()

		<-done
	})

	t.Run("the same name in another bucket IS blocked", func(t *testing.T) {
		t.Parallel()

		other, err := Open(ctx, testutil.FileURL(t, t.TempDir()))
		if err != nil {
			t.Fatalf("opening the second bucket: %v", err)
		}

		// The reverse of what this asserted while the lock was keyed by
		// bucket and key. Two buckets are two directories and it is tempting
		// to conclude they cannot collide; the staging path says otherwise,
		// because it is built from the basename alone and lives in neither
		// bucket. Three t.Parallel tests in ephemeris/jpl/spk are the case.
		release := writeLock(bucket, "one/de440s.bsp")

		blocked := make(chan struct{})

		go func() {
			defer close(blocked)

			writeLock(other, "another/de440s.bsp")()
		}()

		select {
		case <-blocked:
			t.Error("two buckets took the lock for one file name at once.\n" +
				"  Their fileblob staging paths are the same file in os.TempDir, so the " +
				"lock has to exclude them however different the buckets look (#241).")
		default:
		}

		release()
		<-blocked
	})
}
