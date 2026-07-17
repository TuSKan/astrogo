package time

import (
	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/internal/gofaext"
)

// MJD returns the Modified Julian Date (JD - 2400000.5) for t, in
// whatever scale t currently holds.
func (t Time) MJD() float64 {
	return mjdFromJDParts(t.jd1, t.jd2)
}

// GAST returns the Greenwich Apparent Sidereal Time for t (IAU 2006
// model, SOFA Gst06a). If IERS EOP data doesn't cover t's epoch, GAST
// falls back to using UTC in place of UT1 (a few hundred ms of error at
// worst) and returns the lookup error alongside the best-effort result,
// mirroring UT1<->UTC conversion's own fallback contract.
func (t Time) GAST() (angle.Angle, error) {
	ut1, err := t.UT1()
	if err != nil {
		ut1 = t.UTC()
	}

	tt := t.TT()

	u1, u2 := ut1.JDParts()
	tt1, tt2 := tt.JDParts()

	return angle.Rad(gofaext.Gst06a(u1, u2, tt1, tt2)), err
}

// JulianEpochYear returns t's epoch expressed as a Julian-epoch decimal
// year (e.g. J2025.34) — the 2000.0+(JD-2451545.0)/365.25 convention used
// by orbital-element and precession formulas.
func (t Time) JulianEpochYear() float64 {
	return 2000.0 + (t.JD()-2451545.0)/365.25
}

// DayOfYear returns t's fractional day-of-year, with Jan 1 00:00 = 1.0.
func (t Time) DayOfYear() float64 {
	jan1 := Date(t.Year(), 1, 1, 0, 0, 0, 0, t.Location())

	j1a, j1b := jan1.JDParts()
	ta, tb := t.JDParts()

	return (ta - j1a) + (tb - j1b) + 1.0
}
