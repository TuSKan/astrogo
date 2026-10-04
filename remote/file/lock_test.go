package file

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// tests that need a real local filesystem.
func openLocalFS(t *testing.T) (fsys fs.FS, dir string) {
	t.Helper()

	dir = t.TempDir()

	fsys, err := OpenFS(testutil.FileURL(t, dir))
	if err != nil {
		t.Fatalf("OpenFS: %v", err)
	}

	return fsys, dir
}

// TestAcquireLockSerializesAndReleases verifies AcquireLock's own contract
// directly: a second acquire for the same cacheKey blocks until the first
// releases, and succeeds immediately afterward.
func TestAcquireLockSerializesAndReleases(t *testing.T) {
	fsys, _ := openLocalFS(t)

	const cacheKey = "lockfile-test.bin"

	release1, err := AcquireLock(context.Background(), fsys, cacheKey, 0)
	if err != nil {
		t.Fatalf("first AcquireLock: %v", err)
	}

	acquired := make(chan struct{})

	go func() {
		release2, err := AcquireLock(context.Background(), fsys, cacheKey, 0)
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
	fsys, dir := openLocalFS(t)

	const cacheKey = "stale-lock-test.bin"

	if err := WriteFile(t.Context(), fsys, cacheKey+".lock", strings.NewReader("locked")); err != nil {
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

	release, err := AcquireLock(context.Background(), fsys, cacheKey, 0)
	if err != nil {
		t.Fatalf("AcquireLock over a stale lock: %v", err)
	}

	release()
}

// TestAcquireLockHonorsTheCallersStaleAge: a caller that knows how long a live
// holder can take says so, and a lock older than that is stolen, where it used
// to be honored for 30 minutes whatever the download (#445). Zero keeps the 30
// minutes, and so does anything longer: the caller may shorten the wait for a
// crashed holder, not lengthen it.
func TestAcquireLockHonorsTheCallersStaleAge(t *testing.T) {
	for _, c := range []struct {
		name       string
		age        time.Duration
		staleAfter time.Duration
		stolen     bool
	}{
		{"a 2-minute-old lock, stale after a minute", 2 * time.Minute, time.Minute, true},
		{"a 2-minute-old lock, the default", 2 * time.Minute, 0, false},
		{"a 31-minute-old lock, stale after 2 hours", 31 * time.Minute, 2 * time.Hour, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			fsys, dir := openLocalFS(t)

			const cacheKey = "kernel.bin"

			if err := WriteFile(t.Context(), fsys, cacheKey+".lock", strings.NewReader("locked")); err != nil {
				t.Fatalf("seed lock file: %v", err)
			}

			at := time.Now().Add(-c.age)
			if err := os.Chtimes(filepath.Join(dir, cacheKey+".lock"), at, at); err != nil {
				t.Fatalf("backdate lock file: %v", err)
			}

			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()

			release, err := AcquireLock(ctx, fsys, cacheKey, c.staleAfter)
			if c.stolen {
				if err != nil {
					t.Fatalf("AcquireLock: %v, want the lock stolen", err)
				}

				release()

				return
			}

			if err == nil {
				release()
				t.Fatal("AcquireLock stole a lock younger than its stale age")
			}
		})
	}
}
