package file

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/time"
)

// TestAcquireLockAdmitsOneHolderAtATime is the regression.
//
// The existing contract test acquires, then starts a second contender, so
// the second one genuinely finds the lock object already written and the
// check-then-act window inside fileblob is never opened. Simultaneous
// contenders are the case that failed, and nothing exercised it.
//
// AcquireLock delegated exclusion to WriterOptions.IfNotExist, which this
// package and remote/file both documented as being guarded by a per-Bucket
// mutex. It is not: fileblob's bucket struct holds no mutex, and the mutex
// that exists is constructed per writer, so IfNotExist is a bare os.Stat
// followed by an os.Rename. Measured before the fix, 8 goroutines sharing
// one Bucket over 200 rounds: 51 rounds with two or more holders, 2 with
// three (#245).
//
// The assertion is on overlap rather than on a winner count, because
// AcquireLock is not a try-lock — every contender is supposed to get the
// lock eventually. What must never happen is two of them holding it at the
// same moment.
func TestAcquireLockAdmitsOneHolderAtATime(t *testing.T) {
	bucket, _ := openLocalBucket(t)

	const (
		cacheKey   = "exclusion-test.bin"
		contenders = 8
		rounds     = 25
	)

	var (
		inside  atomic.Int32
		overlap atomic.Int32
		peak    atomic.Int32
	)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for range rounds {
		var wg sync.WaitGroup

		for range contenders {
			wg.Go(func() {
				release, err := AcquireLock(ctx, bucket, cacheKey)
				if err != nil {
					t.Errorf("AcquireLock: %v", err)

					return
				}

				defer release()

				n := inside.Add(1)

				for {
					p := peak.Load()
					if n <= p || peak.CompareAndSwap(p, n) {
						break
					}
				}

				if n > 1 {
					overlap.Add(1)
				}

				// Long enough that a second holder would be inside this
				// window rather than slipping through between two
				// instructions.
				time.Sleep(200 * time.Microsecond)

				inside.Add(-1)
			})
		}

		wg.Wait()
	}

	if got := overlap.Load(); got != 0 {
		t.Errorf("%d of %d acquisitions found another holder already inside the lock "+
			"(peak %d simultaneous); AcquireLock is not excluding anyone",
			got, contenders*rounds, peak.Load())
	}
}

// TestAcquireLockReleasesTheInProcessSlotOnFailure covers the leak that
// would turn one failed acquire into a permanently stuck key.
//
// The in-process semaphore is taken before the blob-level loop, so every
// path out of that loop which is not a successful acquire has to hand the
// slot back. If it does not, the next caller for that key waits forever on
// a holder that no longer exists — and waits silently, which is the worst
// version of it.
func TestAcquireLockReleasesTheInProcessSlotOnFailure(t *testing.T) {
	bucket, _ := openLocalBucket(t)

	const cacheKey = "cancelled-acquire-test.bin"

	// The lock object is written directly, standing in for a holder in
	// another process. That is what reaches the path under test: this
	// process takes the in-process slot, finds the object already there,
	// and gives up on its own deadline — so the slot has to come back.
	//
	// Holding it with AcquireLock instead would not do: the second caller
	// would block on the semaphore and fail before ever taking the slot,
	// which exercises nothing. That is exactly how this test first passed
	// against a build that leaked the slot.
	if err := bucket.WriteAll(context.Background(), cacheKey+".lock", []byte("locked"), nil); err != nil {
		t.Fatalf("seed lock object: %v", err)
	}

	blocked, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	if _, err := AcquireLock(blocked, bucket, cacheKey); err == nil {
		t.Fatal("AcquireLock succeeded while another holder's lock object was present")
	}

	if err := bucket.Delete(context.Background(), cacheKey+".lock"); err != nil {
		t.Fatalf("clear lock object: %v", err)
	}

	// If the failed attempt kept the in-process slot, this blocks until the
	// test's own deadline rather than returning.
	done := make(chan struct{})

	go func() {
		defer close(done)

		r, err := AcquireLock(context.Background(), bucket, cacheKey)
		if err != nil {
			t.Errorf("third AcquireLock: %v", err)

			return
		}

		r()
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("AcquireLock blocked after an earlier attempt failed; the in-process " +
			"slot was not handed back")
	}
}

// TestAcquireLockDoesNotSerialiseDifferentKeys keeps the exclusion scoped.
//
// The in-process semaphore is keyed, and a single global one would pass
// every other test here while making a multi-gigabyte kernel download block
// an unrelated bulletin fetch behind it for minutes.
func TestAcquireLockDoesNotSerialiseDifferentKeys(t *testing.T) {
	bucket, _ := openLocalBucket(t)

	first, err := AcquireLock(context.Background(), bucket, "kernel-a.bin")
	if err != nil {
		t.Fatalf("AcquireLock a: %v", err)
	}

	defer first()

	done := make(chan error, 1)

	go func() {
		release, err := AcquireLock(context.Background(), bucket, "kernel-b.bin")
		if err == nil {
			release()
		}

		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("AcquireLock b: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a lock on one key blocked a lock on another; the exclusion is not keyed")
	}
}
