package file_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/remote/file"
)

// TestConcurrentWritersOfOneKeyAllSucceed is the regression for #241, tested at
// the layer the problem lives in.
//
// # What goes wrong without the lock
//
// StageAndPromote writes through a staging object named from the cache key, so
// every concurrent writer of one key aims at the same staging name, and
// fileblob aims each of them at the same temporary file underneath. The loser
// finds the file already renamed away.
//
// Measured here, twelve goroutines writing one key:
//
//	with AcquireLock:     0 failed, 12 succeeded
//	without AcquireLock: 11 failed,  1 succeeded
//
// and the failures carry #241's own message — "Access is denied" renaming
// %TEMP%\<base>.<hex>.tmp onto the staging key.
//
// # Why the assertion is that everyone succeeded
//
// The obvious assertion is that no reader observes truncated or spliced
// content. It is worth nothing here: it passes with the lock removed, because
// the failure mode is a hard error rather than a torn file. A test asserting
// corruption-freedom would have stayed green through the whole of #241, which
// is how this one was written first and why it is not written that way now.
//
// The content checks below stay as a cheap second opinion — each goroutine
// writes its own character repeated to one length, so two writers' bytes in one
// object would be visible — but the count is what discriminates.
//
// # What is not covered
//
// Only the in-process half. AcquireLock's own doc comment explains why the
// mechanism has to be cross-process safe as well, and that half cannot be
// exercised from a single test binary.
func TestConcurrentWritersOfOneKeyAllSucceed(t *testing.T) {
	t.Parallel()

	const (
		key         = "jpl/planets/concurrent.bsp"
		concurrency = 12
		size        = 48 << 10 // large enough for a torn write to land mid-object
	)

	// Goroutine i writes "AAAA...", "BBBB...", and so on: one length,
	// different bytes.
	payloads := make([]string, concurrency)
	for i := range payloads {
		payloads[i] = strings.Repeat(string(rune('A'+i)), size)
	}

	bucket := newBucket(t)

	var (
		mu     sync.Mutex
		errs   []error
		bodies []string
	)

	var wg sync.WaitGroup

	for i := range concurrency {
		wg.Go(func() {
			release, err := file.AcquireLock(t.Context(), bucket, key)
			if err != nil {
				mu.Lock()

				errs = append(errs, err)
				mu.Unlock()

				return
			}

			err = file.StageAndPromote(t.Context(), bucket, key,
				strings.NewReader(payloads[i]), 0, `"etag"`, nil)

			// Read while the lock is still held, so what comes back is what
			// this writer published rather than whatever the next writer has
			// since replaced it with. Reading after the release would make a
			// correct run look like a mixture.
			var got []byte
			if err == nil {
				got, err = bucket.ReadAll(t.Context(), key)
			}

			release()

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				errs = append(errs, err)

				return
			}

			bodies = append(bodies, string(got))
		})
	}

	wg.Wait()

	mu.Lock()
	defer mu.Unlock()

	for _, err := range errs {
		t.Errorf("a writer holding the lock failed: %v.\n"+
			"  Every contender is supposed to get the key in turn and complete. This "+
			"is the failure #241 filed: concurrent writers of one key share a staging "+
			"path and rename it out from under each other.", err)
	}

	if len(bodies) != concurrency {
		t.Fatalf("%d of %d writers completed", len(bodies), concurrency)
	}

	for i, body := range bodies {
		if len(body) != size {
			t.Fatalf("writer %d read %d bytes, want %d — the object was published "+
				"while a writer was still staging it", i, len(body), size)
		}

		if n := strings.Count(body, string(body[0])); n != len(body) {
			t.Fatalf("writer %d read an object holding %d bytes of %q out of %d — "+
				"two writers' content was spliced together", i, n, string(body[0]), len(body))
		}
	}
}
