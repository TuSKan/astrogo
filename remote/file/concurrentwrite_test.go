package file_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote/file"
)

// TestConcurrentWritesOfOneKeySucceed covers a failure that had been showing
// up as three unrelated flaky tests.
//
// fileblob stages a write in the shared os.TempDir under a name built from
// the key's *basename* and the current time, then renames it into the bucket.
// So the staging path does not depend on which bucket is being written, and
// two goroutines caching the same kernel name collide even when their buckets
// are entirely separate — one's staging file gets renamed out from under the
// other, and the loser reports "rename ...: The system cannot find the file
// specified".
//
// The naming assumes nanosecond precision makes that unlikely. On Windows
// time.Now().UnixNano returned one distinct value across 2000 consecutive
// reads — a ~15.6 ms tick — so the assumption does not hold and the O_EXCL
// retry re-reads the same frozen clock. Measured before the fix: 49 failures
// in 320 writes. After: 0.
//
// astrogo does exactly this shape of write. plan's small-body fan-out has
// many goroutines resolving bodies that all need the same base kernel, into
// one cache directory.
//
// The fix is no_tmp_dir=1 in the bucket URL, which [testutil.FileURL] and
// remote's default cache URL both carry, so this test is really asking
// whether they still do.
func TestConcurrentWritesOfOneKeySucceed(t *testing.T) {
	t.Parallel()

	const (
		// The same basename every caller uses; that is the whole point.
		key       = "jpl/planets/de440s.bsp"
		writers   = 8
		rounds    = 20
		wantTotal = writers * rounds
	)

	ctx := context.Background()

	var failures []error

	for round := range rounds {
		// A separate bucket per writer, so nothing but the shared staging
		// directory can bring them into contact.
		buckets := make([]*file.Bucket, writers)

		for i := range writers {
			b, err := file.Open(ctx, testutil.FileURL(t, t.TempDir()))
			if err != nil {
				t.Fatalf("open bucket %d: %v", i, err)
			}

			buckets[i] = b
		}

		errs := make([]error, writers)

		var wg sync.WaitGroup

		for i := range writers {
			wg.Go(func() {
				errs[i] = buckets[i].WriteAll(ctx, key, []byte("kernel bytes"), nil)
			})
		}

		wg.Wait()

		for i, err := range errs {
			if err != nil {
				failures = append(failures, fmt.Errorf("round %d writer %d: %w", round, i, err))
			}
		}
	}

	if len(failures) != 0 {
		t.Errorf("%d of %d concurrent writes of %q failed; the bucket URL is most "+
			"likely missing no_tmp_dir, which puts every writer's staging file in "+
			"one shared directory under one name.\nfirst: %v",
			len(failures), wantTotal, key, errors.Join(failures[:1]...))
	}
}
