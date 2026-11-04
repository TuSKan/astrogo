package time_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/time"
)

// A time whose Julian Date is NaN or infinite has no day. Weekday used to
// convert it to an int anyway: MinInt64 on amd64, which landed on Sunday, and
// 0 on arm64, which landed on Monday (#982).
func TestWeekdayOfANonFiniteTimeIsNoWeekday(t *testing.T) {
	t.Parallel()

	for _, jd := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got := time.FromJD(jd, time.UTC).Weekday(); got != time.NoWeekday {
			t.Errorf("FromJD(%v).Weekday() = %v, want NoWeekday", jd, got)
		}
	}

	if s := time.NoWeekday.String(); s != "" {
		t.Errorf("NoWeekday.String() = %q, want \"\"", s)
	}

	// J2000.0, JD 2451545.0, was a Saturday: a finite time keeps its day.
	if got := time.FromJD(2451545.0, time.UTC).Weekday(); got != time.Saturday {
		t.Errorf("FromJD(2451545.0).Weekday() = %v, want Saturday", got)
	}
}
