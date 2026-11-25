package epoch_test

import (
	"math"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/epoch"
)

// A time whose Julian Date is NaN or infinite has no day. Weekday used to
// convert it to an int anyway: MinInt64 on amd64, which landed on Sunday, and
// 0 on arm64, which landed on Monday (#982).
func TestWeekdayOfANonFiniteTimeIsNoWeekday(t *testing.T) {
	t.Parallel()

	for _, jd := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got := epoch.FromJD(jd, epoch.UTC).Weekday(); got != epoch.NoWeekday {
			t.Errorf("FromJD(%v).Weekday() = %v, want NoWeekday", jd, got)
		}
	}

	// NoWeekday is no day the standard library names.
	if epoch.NoWeekday >= time.Sunday && epoch.NoWeekday <= time.Saturday {
		t.Errorf("NoWeekday = %d, one of the days time.Sunday to time.Saturday", epoch.NoWeekday)
	}

	// J2000.0, JD 2451545.0, was a Saturday: a finite time keeps its day.
	if got := epoch.FromJD(2451545.0, epoch.UTC).Weekday(); got != time.Saturday {
		t.Errorf("FromJD(2451545.0).Weekday() = %v, want Saturday", got)
	}
}
