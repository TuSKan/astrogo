package resolve_test

import (
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestSliceSeqIteration(t *testing.T) {
	targets := []resolve.Target{
		{ID: "1"},
		{ID: "2"},
		{ID: "3"},
	}

	iter := resolve.SliceSeq(targets)

	count := 0

	iter(func(_ resolve.Target, err error) bool {
		testutil.AssertNoError(t, err)

		count++

		return true // Continue iteration
	})
	testutil.AssertEqual(t, "Full iteration", count, 3)

	// Early Abort
	abortCount := 0

	iter(func(_ resolve.Target, _ error) bool {
		abortCount++
		return false // Abort iteration
	})
	testutil.AssertEqual(t, "Early abort", abortCount, 1)
}
