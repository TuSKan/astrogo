package time

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/gofaext"
)

// TestTDBMinusTTAgainstDtdb holds the truncated series to SOFA's iauDtdb,
// which evaluates all 787 terms of Fairhead & Bretagnon (1990). Measured at
// 0.2-day steps the worst differences are 0.79 µs over 1900–2100, 0.92 µs
// over 1600–2400 and 2.7 µs over −1000 to 3000; the coarser steps here keep
// the test fast and can only find less. Until #423 the series was 54 µs off
// iauDtdb within 1900–2100, and at 2026-01-01 read −77.25 µs where iauDtdb
// gives −81.99 µs.
func TestTDBMinusTTAgainstDtdb(t *testing.T) {
	for _, c := range []struct {
		from, to, stepDays, bound float64
	}{
		{1900, 2100, 3.1, 1e-6},
		{1600, 2400, 11.3, 1e-6},
		{-1000, 3000, 57.1, 3e-6},
	} {
		worst, at := 0.0, 0.0

		for jd := 2451545.0 + (c.from-2000)*365.25; jd < 2451545.0+(c.to-2000)*365.25; jd += c.stepDays {
			jd1 := math.Floor(jd)

			if d := math.Abs(tdbMinusTT(jd1, jd-jd1) - gofaext.Dtdb(jd1, jd-jd1, 0, 0, 0, 0)); d > worst {
				worst, at = d, jd
			}
		}

		if worst > c.bound {
			t.Errorf("%v to %v: TDB−TT %.2f µs from iauDtdb at JD %.1f, want within %.0f µs",
				c.from, c.to, worst*1e6, at, c.bound*1e6)
		}
	}

	// The issue's instant: 2026-01-01 00:00 UTC, TT JD 2461041.500800741.
	if got, want := tdbMinusTT(2461041.5, 0.000800741), gofaext.Dtdb(2461041.5, 0.000800741, 0, 0, 0, 0); math.Abs(got-want) > 1e-6 {
		t.Errorf("TDB−TT at 2026-01-01 %.2f µs, iauDtdb gives %.2f µs", got*1e6, want*1e6)
	}
}
