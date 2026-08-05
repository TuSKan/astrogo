package remote

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gofs "github.com/ungerik/go-fs"
)

// TestGetFileConcurrentSameDestNoCorruption is a regression test for a real
// cross-process race the resumable-download feature introduced: writePartial
// writes to a FIXED-name .part file per destination, so two callers that
// both observe a cache miss and both proceed to download corrupt each
// other's write to that one file — this is exactly how it was first caught:
// `go test ./...` runs each package as its own OS process, several of which
// fetch the same shared JPL kernel, and CI failed with a truncated SPK
// kernel of a DIFFERENT size on every run.
//
// This test only exercises the intra-process (goroutine) half of the fix —
// acquireLock's own doc comment explains why the mechanism has to be
// cross-process safe, not just intra-process, and that half is inherently
// untestable from a single `go test` binary.
func TestGetFileConcurrentSameDestNoCorruption(t *testing.T) {
	cleanRemoteState(t)

	// Large enough that a missing lock would corrupt some goroutine's read
	// under Go's scheduler — the lock makes the outcome deterministic
	// either way, this size just keeps the test meaningful if the fix is
	// ever accidentally reverted.
	payload := strings.Repeat("kernel-data-", 4096) // ~48 KB

	var hits atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)

		_, _ = w.Write([]byte(payload))
	}))
	defer srv.Close()

	if err := SetURL(NAIFSPK, srv.URL); err != nil {
		t.Fatal(err)
	}

	EnableDownloads(NAIFSPK, 0)

	const concurrency = 12

	var wg sync.WaitGroup

	errs := make([]error, concurrency)
	contents := make([]string, concurrency)

	for i := range concurrency {
		wg.Go(func() {
			f, err := GetFile(context.Background(), NAIFSPK, "planets/concurrent.bsp")
			if err != nil {
				errs[i] = err

				return
			}

			data, err := f.ReadAll()
			if err != nil {
				errs[i] = err

				return
			}

			contents[i] = string(data)
		})
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: GetFile: %v", i, err)
		}

		if contents[i] != payload {
			t.Fatalf("goroutine %d: content corrupted — got %d bytes, want %d", i, len(contents[i]), len(payload))
		}
	}

	// A stronger check than "no corruption": the lock should have
	// serialized every caller onto the SAME download rather than merely
	// preventing them from clobbering each other's bytes.
	if got := hits.Load(); got != 1 {
		t.Errorf("lock should have serialized the download to exactly 1 request; server saw %d", got)
	}
}

// TestAcquireLockSerializesAndReleases verifies acquireLock's own contract
// directly: a second acquire for the same dest blocks until the first
// releases, and succeeds immediately afterward.
func TestAcquireLockSerializesAndReleases(t *testing.T) {
	dest := gofs.File(t.TempDir()).Join("lockfile-test.bin")

	release1, ok, err := acquireLock(context.Background(), dest)
	if err != nil {
		t.Fatalf("first acquireLock: %v", err)
	}

	if !ok {
		t.Fatal("first acquireLock: ok = false, want true for a local path")
	}

	acquired := make(chan struct{})

	go func() {
		release2, ok, err := acquireLock(context.Background(), dest)
		if err != nil {
			t.Errorf("second acquireLock: %v", err)

			return
		}

		if !ok {
			t.Error("second acquireLock: ok = false, want true")
		}

		release2()

		close(acquired)
	}()

	select {
	case <-acquired:
		t.Fatal("second acquireLock returned before the first was released")
	default:
	}

	release1()

	<-acquired // must complete now that the lock is free
}

// TestAcquireLockStealsAbandonedLock verifies a lock file older than
// staleLockAge is treated as abandoned rather than honored forever — the
// safety net for a holder that crashed mid-download.
func TestAcquireLockStealsAbandonedLock(t *testing.T) {
	dest := gofs.File(t.TempDir()).Join("stale-lock-test.bin")

	lock := lockPath(dest)

	if err := lock.WriteAll(nil); err != nil {
		t.Fatalf("seed lock file: %v", err)
	}

	// Back-date it past staleLockAge instead of waiting 30 real minutes.
	// gofs.File has no setter for mtime, so this drops to the raw OS path
	// exactly as acquireLock itself does.
	stale := time.Now().Add(-(staleLockAge + time.Minute))
	if err := os.Chtimes(lock.LocalPath(), stale, stale); err != nil {
		t.Fatalf("backdate lock file: %v", err)
	}

	release, ok, err := acquireLock(context.Background(), dest)
	if err != nil {
		t.Fatalf("acquireLock over a stale lock: %v", err)
	}

	if !ok {
		t.Fatal("ok = false, want true")
	}

	release()
}
