package file

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
)

// tests that need a real local Bucket.
func openLocalBucket(t *testing.T) (bucket *Bucket, dir string) {
	t.Helper()

	dir = t.TempDir()

	url := testutil.FileURL(t, dir)

	bucket, err := Open(context.Background(), url)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	return bucket, dir
}

// TestAcquireLockSerializesAndReleases verifies AcquireLock's own contract
// directly: a second acquire for the same cacheKey blocks until the first
// releases, and succeeds immediately afterward.
func TestAcquireLockSerializesAndReleases(t *testing.T) {
	bucket, _ := openLocalBucket(t)

	const cacheKey = "lockfile-test.bin"

	release1, err := AcquireLock(context.Background(), bucket, cacheKey)
	if err != nil {
		t.Fatalf("first AcquireLock: %v", err)
	}

	acquired := make(chan struct{})

	go func() {
		release2, err := AcquireLock(context.Background(), bucket, cacheKey)
		if err != nil {
			t.Errorf("second AcquireLock: %v", err)

			return
		}

		release2()

		close(acquired)
	}()

	select {
	case <-acquired:
		t.Fatal("second AcquireLock returned before the first was released")
	default:
	}

	release1()

	<-acquired // must complete now that the lock is free
}

// TestAcquireLockStealsAbandonedLock verifies a lock file older than
// staleLockAge is treated as abandoned rather than honored forever — the
// safety net for a holder that crashed mid-download.
func TestAcquireLockStealsAbandonedLock(t *testing.T) {
	bucket, dir := openLocalBucket(t)

	const cacheKey = "stale-lock-test.bin"

	if err := bucket.WriteAll(context.Background(), cacheKey+".lock", []byte("locked"), nil); err != nil {
		t.Fatalf("seed lock file: %v", err)
	}

	// Back-date it past staleLockAge instead of waiting 30 real minutes.
	// fileblob has no metadata setter for mtime through the Bucket API, so
	// this drops to the raw OS path exactly as AcquireLock's own
	// Attributes(ctx, lockKey).ModTime check does under the hood.
	stale := time.Now().Add(-(staleLockAge + time.Minute))
	if err := os.Chtimes(filepath.Join(dir, cacheKey+".lock"), stale, stale); err != nil {
		t.Fatalf("backdate lock file: %v", err)
	}

	release, err := AcquireLock(context.Background(), bucket, cacheKey)
	if err != nil {
		t.Fatalf("AcquireLock over a stale lock: %v", err)
	}

	release()
}
