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
// holder of the same key exists, and a different key is not blocked by it.
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

	t.Run("a different key is not blocked", func(t *testing.T) {
		t.Parallel()

		// Held for the duration, so a lock on another key that waited for it
		// would hang the test rather than fail it — which is the honest
		// outcome for a deadlock and is what a timeout reports.
		release := writeLock(bucket, "held/key")
		defer release()

		done := make(chan struct{})

		go func() {
			defer close(done)

			writeLock(bucket, "other/key")()
		}()

		<-done
	})

	t.Run("the same key in another bucket is not blocked", func(t *testing.T) {
		t.Parallel()

		other, err := Open(ctx, testutil.FileURL(t, t.TempDir()))
		if err != nil {
			t.Fatalf("opening the second bucket: %v", err)
		}

		release := writeLock(bucket, "shared/name")
		defer release()

		done := make(chan struct{})

		go func() {
			defer close(done)

			writeLock(other, "shared/name")()
		}()

		<-done
	})
}
