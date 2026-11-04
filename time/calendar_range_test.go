package time_test

import (
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/time"
)

var errPanicked = errors.New("panicked")

// SOFA's calendar conversion stops at JD −68569.5 (−4900 March 1) and JD 1e9,
// and beyond them it gave year 0, month 0, day 0. DecimalYear indexed a table
// with that month, so Time.TT panicked for every UTC instant before −4900, and
// every ephemeris and coordinate path with it (#961).

// noPanic runs f and reports a panic as an error rather than crashing the test.
func noPanic(f func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", errPanicked, r)
		}
	}()

	f()

	return nil
}

// TestConversionsSurviveEveryJulianDate: Calendar, DecimalYear, TT, TDB, UTC
// and DeltaT return for any Julian Date, inside SOFA's range, either side of
// it, far beyond, and not finite at all. A finite one has a calendar date, and
// one that is not has no fractional year but its own non-finite value.
//
// Earth orientation is pinned to zero: an instant outside the bulletin would
// otherwise start the lazy load, which is not what this test is about.
func TestConversionsSurviveEveryJulianDate(t *testing.T) {
	time.RegisterModel(time.ZeroModel{})
	t.Cleanup(time.ResetEOP)

	for _, jd := range []float64{
		-68569.5, -68570, -1e6, -1e12, 2451545, 1e9, 1e9 + 1, 1e12, math.NaN(), math.Inf(1), math.Inf(-1),
	} {
		tm := time.FromJD(jd, time.UTC)

		for name, f := range map[string]func(){
			"Calendar":    func() { _, _, _, _ = tm.Calendar() },
			"DecimalYear": func() { _ = tm.DecimalYear() },
			"TT":          func() { _ = tm.TT() },
			"TDB":         func() { _ = tm.TDB() },
			"UTC":         func() { _ = tm.UTC() },
			"DeltaT":      func() { _ = time.DeltaT(tm) },
		} {
			if err := noPanic(f); err != nil {
				t.Errorf("JD %g: %s: %v", jd, name, err)
			}
		}

		finite := !math.IsNaN(jd) && !math.IsInf(jd, 0)

		y, m, d, _ := tm.Calendar()
		dy := tm.DecimalYear()

		switch {
		case finite && (m < 1 || m > 12 || d < 1 || d > 31):
			t.Errorf("JD %g: Calendar %d-%d-%d, a finite Julian Date with no date", jd, y, m, d)
		case finite && (math.IsNaN(dy) || math.IsInf(dy, 0)):
			t.Errorf("JD %g: DecimalYear %v for a finite Julian Date", jd, dy)
		case !finite && !math.IsNaN(dy) && !math.IsInf(dy, 0):
			t.Errorf("JD %g: DecimalYear %v, a finite year for a Julian Date that is not", jd, dy)
		}
	}
}

// TestDecimalYearRisesAcrossSOFAsLimits: half a day either side of each end
// of SOFA's range, the fractional year moves forward with the date, as it
// would if SOFA's range were not there.
func TestDecimalYearRisesAcrossSOFAsLimits(t *testing.T) {
	for _, edge := range []float64{-68569.5, 1e9} {
		before := time.FromJD(edge-0.5, time.UTC).DecimalYear()
		at := time.FromJD(edge, time.UTC).DecimalYear()
		after := time.FromJD(edge+0.5, time.UTC).DecimalYear()

		if before >= at || at >= after {
			t.Errorf("around JD %g: DecimalYear %.9f, %.9f, %.9f: not rising with the date", edge, before, at, after)
		}

		if step := (after - before) * 365.2425; math.Abs(step-1) > 0.01 {
			t.Errorf("around JD %g: a day moves DecimalYear by %.6f days' worth, want 1", edge, step)
		}
	}
}
