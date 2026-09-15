package file_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/remote/file"
)

// TestStagingAndPartialWritesAcrossBuckets exercises the basename collision
// seen in the Windows dependency-update CI. AcquireLock sees different cache
// keys here, while fileblob stages both writes as shared.bsp.part in os.TempDir.
// SavePartial and StageAndPromote must share the same write exclusion.
func TestStagingAndPartialWritesAcrossBuckets(t *testing.T) {
	t.Parallel()

	const (
		writers = 8
		rounds  = 8
	)

	payload := strings.Repeat("staged kernel bytes ", 32_000)

	for round := range rounds {
		buckets := make([]*file.Bucket, writers)
		for i := range writers {
			buckets[i] = newBucket(t)
		}

		errs := make([]error, writers)
		start := make(chan struct{})

		var wg sync.WaitGroup

		for i := range writers {
			wg.Go(func() {
				<-start

				key := fmt.Sprintf("writer-%d/shared.bsp", i)
				if i%2 == 0 {
					errs[i] = file.SavePartial(t.Context(), buckets[i],
						file.PartialKey(key), strings.NewReader(payload), "etag")

					return
				}

				release, err := file.AcquireLock(t.Context(), buckets[i], key)
				if err != nil {
					errs[i] = err

					return
				}

				defer release()

				err = file.StageAndPromote(t.Context(), buckets[i], key,
					strings.NewReader(payload), 0, "etag",
					func(_ io.Reader) error { return errValidationFailed })
				if !errors.Is(err, errValidationFailed) {
					t.Errorf("round %d, writer %d: StageAndPromote = %v, want validation rejection",
						round, i, err)
				}
			})
		}

		close(start)
		wg.Wait()

		for i, err := range errs {
			if err != nil {
				t.Errorf("round %d, writer %d: %v", round, i, err)
			}
		}
	}
}

// TestAcquireLockReportsACancelledContext covers the path where the lock
// object cannot be created for a reason that is not contention.
//
// The three codes a losing writer produces — FailedPrecondition, Unknown and
// NotFound — are all retried, which is what makes the lock work under
// contention and what makes this path easy to leave untested. A cancelled
// context is the case that must *not* be retried: the caller has gone, and
// spinning until the deadline would be the one outcome worse than failing.
func TestAcquireLockReportsACancelledContext(t *testing.T) {
	t.Parallel()

	bucket, err := file.Open(t.Context(), "file:///"+filepath.ToSlash(t.TempDir())+"?create_dir=true")
	if err != nil {
		t.Fatalf("open bucket: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	release, err := file.AcquireLock(ctx, bucket, "cancelled.bsp")
	if release != nil {
		release()
	}

	if err == nil {
		t.Fatal("a cancelled context produced a lock; the caller is gone and " +
			"nothing should be holding one on its behalf")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("AcquireLock with a cancelled context returned %v, want it to "+
			"wrap context.Canceled rather than be retried as contention", err)
	}

	t.Logf("reported: %v", err)
}
