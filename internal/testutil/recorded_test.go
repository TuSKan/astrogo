package testutil_test

import (
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestRecordedStartsAtTheZeroValue(t *testing.T) {
	t.Parallel()

	var r testutil.Recorded[string]

	if got := r.Get(); got != "" {
		t.Errorf("Get before any Set = %q, want the zero value", got)
	}
}

func TestRecordedKeepsTheLastValue(t *testing.T) {
	t.Parallel()

	var r testutil.Recorded[int]

	r.Set(1)
	r.Set(2)

	if got := r.Get(); got != 2 {
		t.Errorf("Get = %d, want the last value set, 2", got)
	}
}

// The reason Recorded exists: sets from several goroutines while another
// reads. Under -race a plain variable fails this; Recorded must not.
func TestRecordedIsSafeAcrossGoroutines(t *testing.T) {
	t.Parallel()

	var (
		r  testutil.Recorded[string]
		wg sync.WaitGroup
	)

	for range 8 {
		wg.Go(func() {
			for range 100 {
				r.Set("handler")
				_ = r.Get()
			}
		})
	}

	wg.Wait()

	if got := r.Get(); got != "handler" {
		t.Errorf("Get = %q, want %q", got, "handler")
	}
}
