package file_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/remote/file"
)

// TestStagingAndPartialWritesAcrossFilesystems is a regression test for a
// defect that no longer has a mechanism, kept because the property it asserts
// is the one that matters.
//
// It was written for the basename collision seen in the Windows
// dependency-update CI: AcquireLock saw different cache keys here, while
// fileblob staged every write as shared.bsp.<clock>.tmp in os.TempDir — one
// name, because that clock does not advance on Windows — so eight writers of
// eight different keys in eight different buckets fought over one file. The fix
// at the time was a process-wide lock keyed on the basename.
//
// Both the collision and that lock are gone. Staging is now named from the
// process id and an atomic counter and happens inside the tree, so two writers
// cannot pick one name however their keys are spelled. What is left is the
// assertion: concurrent staged writes to distinct filesystems all succeed, and
// a validator's rejection reaches the caller rather than a name conflict.
func TestStagingAndPartialWritesAcrossFilesystems(t *testing.T) {
	t.Parallel()

	const (
		writers = 8
		rounds  = 8
	)

	payload := strings.Repeat("staged kernel bytes ", 32_000)

	for round := range rounds {
		filesystems := make([]fs.FS, writers)
		for i := range writers {
			filesystems[i] = newFS(t)
		}

		errs := make([]error, writers)
		start := make(chan struct{})

		var wg sync.WaitGroup

		for i := range writers {
			wg.Go(func() {
				<-start

				key := fmt.Sprintf("writer-%d/shared.bsp", i)
				if i%2 == 0 {
					errs[i] = file.WriteFile(t.Context(), filesystems[i],
						file.PartialKey(key), strings.NewReader(payload))

					return
				}

				release, err := file.AcquireLock(t.Context(), filesystems[i], key)
				if err != nil {
					errs[i] = err

					return
				}

				defer release()

				err = file.StageAndPromote(t.Context(), filesystems[i], key,
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

	fsys, err := file.OpenFS("file:///" + filepath.ToSlash(t.TempDir()) + "?create_dir=true")
	if err != nil {
		t.Fatalf("open bucket: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	release, err := file.AcquireLock(ctx, fsys, "cancelled.bsp")
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
