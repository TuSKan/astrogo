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
// falls back to using UTC in place of UT1 and returns the lookup error
// alongside the best-effort result, mirroring UT1<->UTC conversion's own
// fallback contract.
//
// The error that fallback costs is bounded only by the leap-second system:
// UT1-UTC is held inside ±0.9 s, with IERS acting near ±0.7 s in practice.
// So the worst case is about 0.9 s of Earth rotation — roughly 13.5 arcsec,
// or 420 m of ground position at the equator — not the "few hundred ms" this
// comment claimed before the bound was checked.
//
// That bound held only while leap seconds did. Resolution 5 of the 28th CGPM
// (2026) makes UTC continuous from 20 May 2027 and raises the limit on
// |UT1-UTC| to 3600 s, so from that date this fallback drifts with UT1-UTC
// itself (#879). Callers that silently discard the returned error will drift
// silently with it.
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

// DayOfYear returns t's fractional day-of-year, with Jan 1 00:00 = 1.0, in
// the calendar [Time.Year] reads: t's own scale, whatever its display
// location.
//
// It used to measure from Jan 1 midnight in the display location while
// taking the year from Year, which ignores it, so near the new year the two
// described different dates: 01:00 UTC on 1 January 2026 shown in São Paulo
// came out 0.917, and 23:00 UTC on 31 December shown in Tokyo 366.33 in a
// 365-day year (#760). A caller pairing Year with DayOfYear, as a TLE epoch
// does, needs both from one date.
func (t Time) DayOfYear() float64 {
	y, m, d, frac := t.Calendar()

	return float64(mjdOf(y, m, d)-mjdOf(y, 1, 1)) + 1 + frac
}
