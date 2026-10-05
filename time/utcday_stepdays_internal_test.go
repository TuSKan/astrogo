package time

import (
	"testing"

	"github.com/TuSKan/astrogo/internal/gofaext"
)

// TestStepDaysCoverTheIndex: stepDays is a cheap day-of-month filter consulted
// before the step index, so a step day whose day of the month is missing from
// it is skipped outright — its step never applied. #479 added the 1960s jumps,
// which end the 28th, 30th and 31st, and this holds the mask to the index
// rather than to the comment that lists them.
func TestStepDaysCoverTheIndex(t *testing.T) {
	t.Parallel()

	steps := currentUTCSteps()
	if len(steps.leap) == 0 {
		t.Fatal("the step index is empty")
	}

	rubber := 0

	for mjd, leap := range steps.leap {
		_, _, d, _, _ := gofaext.JdToDate(mjdZero+float64(mjd), 0.5)
		if stepDays()&(1<<uint(d)) == 0 {
			t.Errorf("MJD %d ends in a step of %v s on day %d of its month, which stepDays leaves out", mjd, leap, d)
		}

		if leap != 1 && leap != -1 {
			rubber++
		}
	}

	if rubber < 10 {
		t.Errorf("only %d fractional steps indexed; SOFA's 1960s table has more", rubber)
	}
}
