package file_test

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote/file"
)

// TestConcurrentSaveOfOneNameDoesNotCollide is the regression test for #241.
//
// # What used to happen
//
// fileblob stages every write in os.TempDir under a name built from the key's
// basename and time.Now().UnixNano() in hex, on the reasoning that the clock
// "changes enough between each iteration to make a conflict unlikely". On
// Windows the clock granularity is about 15.6 ms and 2000 consecutive reads
// returned one distinct value, so concurrent writers agree on the staging path
// and one renames it out from under another. The failure arrives as
//
//	rename ...\de440s.bsp.18d3947f1dea1bb4.tmp ...: The system cannot find the file specified
//
// against a key nothing else touched. Measured on main before the fix, eight
// writers over forty rounds: 31 to 69 failures out of 320.
//
// # Why separate buckets
//
// Because that is the case an obvious fix misses. The staging path contains no
// part of the bucket, so two writers of the same file name collide even when
// their buckets are different directories — which is how this first showed up,
// in three t.Parallel tests that each had their own t.TempDir. A lock keyed on
// the bucket and key would leave exactly this failing, so this is the shape
// worth pinning.
//
// The shared-bucket case is covered too, by the same lock, and is not repeated
// here: the cross-process half of it belongs to remote's download lock (#245),
// and a test with both would not say which one held.
//
// # It can only fail on Windows
//
// Elsewhere the nanosecond clock does move and the staging names differ, so
// this passes whether or not the lock exists. That is worth stating rather
// than leaving the next reader to wonder why a race test is not flaky: it is a
// platform regression guard, and CI runs Windows.
func TestConcurrentSaveOfOneNameDoesNotCollide(t *testing.T) {
	t.Parallel()

	// The real collision: several goroutines caching one kernel name, which
	// is what plan's small-body fan-out does on every query.
	const (
		key     = "jpl/planets/de440s.bsp"
		writers = 8
		rounds  = 20
	)

	root := t.TempDir()

	for round := range rounds {
		buckets := make([]*file.Bucket, writers)

		for i := range writers {
			dir := filepath.Join(root, string(rune('a'+i)), string(rune('a'+round%26)))

			b, err := file.Open(t.Context(), testutil.FileURL(t, dir))
			if err != nil {
				t.Fatalf("open bucket %d: %v", i, err)
			}

			buckets[i] = b
		}

		errs := make([]error, writers)

		var wg sync.WaitGroup

		for i := range writers {
			wg.Go(func() {
				errs[i] = file.Save(t.Context(), buckets[i], key,
					strings.NewReader("kernel bytes"))
			})
		}

		wg.Wait()

		for i, err := range errs {
			if err != nil {
				t.Fatalf("round %d, writer %d: %v.\n"+
					"  Concurrent writers of one file name share fileblob's staging path, "+
					"and one renames it out from under another. file.Save holds the lock "+
					"that serialises them (#241).", round, i, err)
			}
		}
	}
}

// TestLockStagingIsPerName checks that the fix does not serialise everything.
//
// A single global write lock would also make this test's failure impossible,
// and would turn every concurrent cache write in the process into a queue.
// Distinct names have to stay independent, which is the whole reason the lock
// is keyed rather than global.
//
// It is asserted structurally rather than by timing: two locks are taken for
// different names, and if they excluded each other the second acquisition would
// deadlock and the test would time out rather than fail confusingly. The same
// name is then checked to be exclusive, which is the property the regression
// above depends on.
func TestLockStagingIsPerName(t *testing.T) {
	t.Parallel()

	// Different basenames, and deliberately in the same directory, so nothing
	// but the name distinguishes them.
	releaseA := file.LockStaging("jpl/planets/de440s.bsp")
	releaseB := file.LockStaging("jpl/planets/de441.bsp")

	releaseB()
	releaseA()

	// Same basename from two different key paths: still one lock, because the
	// staging path fileblob builds is the same for both.
	release := file.LockStaging("one/dir/de440s.bsp")

	held := make(chan struct{})

	go func() {
		defer file.LockStaging("another/dir/de440s.bsp")()

		close(held)
	}()

	select {
	case <-held:
		t.Error("two keys with the same basename acquired the staging lock at once.\n" +
			"  Their fileblob staging paths are identical — os.TempDir plus that basename " +
			"— so they must exclude each other however different the keys look (#241).")
	default:
	}

	release()
	<-held
}
