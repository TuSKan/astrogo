package plan

import (
	"fmt"
	"math"
	"strings"

	"github.com/TuSKan/astrogo/time"
)

// ── Lunar Crescent Visibility Criteria ───────────────────────────────────────
//
// Eighteen published criteria for the first visibility of the young lunar
// crescent, 1910–2021, each a pure function of CrescentParams returning a
// verdict or a zone.
//
// The criteria were not defined in one convention. Each author computed his
// quantities in his own way and at his own instant, and a criterion fed
// another's moves its answer: Yallop's q read a topocentric arc of vision
// 0.1 low, a whole zone near a boundary (#496). So a method reads its
// CrescentParams as given, its doc names the convention and the instant the
// author used, and CrescentVisibility computes each criterion's parameters in
// that convention. Every formula, coefficient and table here is taken from
// the criterion's own publication, cited on its method (#503):
//
//	criterion          quantities                                   instant
//	Fotheringham 1910  geocentric Moon altitude, DAZ                geometric sunset
//	Maunder 1911       geocentric Moon altitude, DAZ                geometric sunset
//	Ilyas 1988         Moon altitude, DAZ                           geometric sunset
//	Krauss 2012        geocentric Moon altitude, DAZ                geometric sunset
//	Ilyas 1983         geocentric elongation                        geometric sunset
//	Danjon 1932        topocentric elongation                       sunset
//	Fatoohi 1998       topocentric elongation                       sunset
//	MABIMS 1995, 2021  topocentric altitude, geocentric elongation  sunset
//	Istanbul 2016      altitude and elongation, as MABIMS           sunset
//	Alrefay 2018       topocentric ARCV, W from a 16′ semi-diameter sunset
//	Caldwell 2001      apparent lower-limb altitude, DAZ            sunset
//	Bruin 1977         geocentric ARCV, W from a 15′ semi-diameter  Yallop's best time
//	Yallop 1997        geocentric ARCV, topocentric width W′        Yallop's best time
//	Odeh 2004          topocentric ARCV and W                       Yallop's best time
//	Qureshi 2010       geocentric ARCV, topocentric width W′        Qureshi's best time
//
// Geometric sunset is when the Sun's center is on the geometric horizon, its
// altitude 0°, a few minutes before the sunset of an almanac, which is the
// upper limb on the refracted horizon. All angles are airless unless named
// apparent.

// CrescentParams holds the quantities a crescent visibility criterion reads.
//
// Which positions they are taken from, and at which instant, depends on the
// criterion: each method's doc says, and CrescentVisibility fills one set per
// criterion, which CrescentVerdict and CrescentZone carry back.
type CrescentParams struct {
	// ArcV is the arc of vision, ARCV: the Moon's altitude minus the Sun's,
	// in degrees.
	ArcV float64

	// ArcL is the arc of light, ARCL: the elongation of the Moon from the
	// Sun, center to center, in degrees.
	ArcL float64

	// DAZ is the difference in azimuth between the Sun and the Moon, in
	// degrees, unsigned.
	DAZ float64

	// MAlt is the Moon's altitude, in degrees.
	MAlt float64

	// W is the width of the crescent, in arcminutes.
	W float64

	// LT is the lag: moonset minus sunset, in minutes.
	LT float64

	// Age is the Moon's age, the time since the last new moon, in hours.
	Age float64
}

// ── Multi-Zone Classification ────────────────────────────────────────────────

// CrescentZone represents a visibility classification zone returned by
// multi-zone criteria (Yallop, Odeh, Qureshi).
type CrescentZone struct {
	// Code is the short classification identifier (e.g. "A", "Naked Eye").
	Code string

	// Label is the full human-readable description
	// (e.g. "Easily visible", "May need optical aid").
	Label string

	// Value is the computed discriminant parameter (q, V, or s).
	Value float64

	// Params are the quantities the zone was computed from.
	Params CrescentParams
}

// String returns a formatted representation: "Code: Label (value=X.XXXX)".
func (z CrescentZone) String() string {
	return fmt.Sprintf("%s: %s (value=%.4f)", z.Code, z.Label, z.Value)
}

// CrescentVerdict is a visible-or-not criterion's answer, with the quantities
// it read.
type CrescentVerdict struct {
	// Params are the quantities the criterion read, in its own convention.
	Params CrescentParams

	// Visible is the criterion's verdict.
	Visible bool
}

// ── Azimuth–altitude criteria: the Moon's altitude at sunset against DAZ ────
//
// The oldest family: the Moon's altitude when the Sun's center is on the
// horizon, against the difference in azimuth. With the Sun at altitude 0 the
// Moon's altitude is also the arc of vision (Krauss 2012).

// Fotheringham evaluates Fotheringham's (1910) criterion: the Moon must stand
// at least 12.0° − 0.008°·DAZ² above the horizon.
//
// Fotheringham, J. K. (1910), MNRAS 70, 527, p. 531, "Minimum Altitude =
// 12°·0 − 0°·008 Z²", fitted to Julius Schmidt's and Mommsen's observations
// at Athens. The altitude is the Moon's "true altitude at sunset", computed
// without the lunar parallax (p. 528): geocentric and airless, at geometric
// sunset. Reads MAlt and DAZ.
//
// He calls the formula a rough approximation of the table on his p. 530,
// which runs only to DAZ 23°.
func (p *CrescentParams) Fotheringham() bool {
	return p.MAlt >= 12.0-0.008*p.DAZ*p.DAZ
}

// Maunder evaluates Maunder's (1911) criterion: the Moon must stand at least
// 11° − DAZ/20 − DAZ²/100 above the horizon.
//
// Maunder, E. W. (1911), JBAA 21, 355, revised Fotheringham's line on the
// same observations. His table (p. 359: 11.0, 10.5, 9.5, 8.0, 6.0° at DAZ 0,
// 5, …, 20°) is fitted exactly by this quadratic (Yallop 1997, NAO TN 69,
// Table 1 and eq. 3.1; Krauss 2012). The quantities are Fotheringham's: the
// Moon's geocentric altitude at geometric sunset. Reads MAlt and DAZ.
func (p *CrescentParams) Maunder() bool {
	d := math.Abs(p.DAZ)

	return p.MAlt >= 11.0-d/20-d*d/100
}

// ilyas1988Form is the curve of Ilyas (1988), A&A 206, 133, Fig. 5 (Form B):
// the Moon's altitude at sunset against DAZ, every 2.5° from 0° to 60°.
//
// Ilyas published it only as a figure. These values were traced from the
// scan of Fig. 5, 45 pixels to the degree, and are good to about 0.1°; his
// text puts the curve's DAZ 0 end at the 10.4° lowest elongation, 0.1 above
// the trace.
var ilyas1988Form = []float64{
	10.29, 10.12, 9.90, 9.60, 9.21, 8.68, 7.95, 7.22, 6.42, 5.73, 5.18, 4.77, 4.51,
	4.34, 4.25, 4.22, 4.23, 4.19, 4.17, 4.15, 4.13, 4.11, 4.09, 4.10, 4.12,
}

// Ilyas1988 evaluates Ilyas's (1988) criterion: the Moon's altitude at sunset
// must reach his curve of altitude against DAZ, which levels off at about 4°
// beyond DAZ 35° — the limiting altitude separation of his title.
//
// It extends Fotheringham's and Maunder's criterion to large DAZ, evaluated
// at sunset with the Sun's altitude 0 (his Appendix), and is read in their
// convention: the Moon's geocentric altitude at geometric sunset. Ilyas does
// not restate it. Reads MAlt and DAZ. Beyond DAZ 60°, the end of his curve,
// its last value holds.
func (p *CrescentParams) Ilyas1988() bool {
	return p.MAlt >= interpolateUniform(ilyas1988Form, 2.5, math.Abs(p.DAZ))
}

// krauss2012Athens and krauss2012AthensDAZ are the Athenian column of Krauss
// (2012), Table 13: the crescent altitude h* at the middle of the zone where
// a sighting becomes likely, against DAZ, for March to September.
var (
	krauss2012AthensDAZ = []float64{0, 5, 10, 15, 20, 22}
	krauss2012Athens    = []float64{10.6, 10.5, 9.95, 9.0, 7.6, 7.0}
)

// KraussAthenian evaluates Krauss's (2012) Athenian criterion for the warm
// season: the crescent is visible if the Moon's altitude reaches h*.
//
// Krauss, R. (2012), PalArch's J. Archaeol. Egypt/Egyptol. 9(5), Table 13,
// from J. Schmidt's and others' observations at Athens. h* is the middle of
// an uncertainty zone ±1.8° wide at DAZ 0: near its lower edge Krauss puts
// the chance of a sighting as slight, at h* as medium, near its upper edge as
// sizeable; this reads h* as the line. The column is for March to September,
// and Krauss leaves the change of season to the user.
//
// The altitude is the Moon's geocentric altitude when the Sun's geocentric
// altitude is 0°, as Krauss defines it after Fotheringham. Reads MAlt and DAZ.
// Beyond DAZ 22°, the end of the table, its last value holds.
func (p *CrescentParams) KraussAthenian() bool {
	return p.MAlt >= interpolateTable(krauss2012AthensDAZ, krauss2012Athens, math.Abs(p.DAZ))
}

// ── Elongation limits ───────────────────────────────────────────────────────

// Danjon evaluates the Danjon limit: no crescent is visible within 7° of the
// Sun.
//
// Danjon (1932, L'Astronomie 46, 57; 1936) extrapolated the shortening of the
// crescent to zero length at an elongation of 7°, the elongation "taking
// account of lunar parallax" — topocentric (Danjon 1932, p. 60, as translated
// by Fatoohi, Stephenson & Al-Dargazelli 1998, Observatory 118, 65). Schaefer
// (1991, QJRAS 32, 265) confirms the 7°. Read at sunset. Reads ArcL.
func (p *CrescentParams) Danjon() bool {
	return p.ArcL >= 7.0
}

// Fatoohi1998 evaluates the limit of Fatoohi, Stephenson & Al-Dargazelli
// (1998, Observatory 118, 65): no crescent is visible within 7.5° of the Sun.
//
// The smallest elongation among 503 ancient and modern sightings was 7.5°,
// computed at sunset "allowing for parallax" (their Table I) — topocentric.
// Reads ArcL.
func (p *CrescentParams) Fatoohi1998() bool {
	return p.ArcL >= 7.5
}

// Ilyas1983 evaluates the limit of Ilyas (1983, JRASC 77, 214): no crescent
// is visible within 10.5° of the Sun.
//
// The lowest elongation of the composite Maunder–Bruin criterion at local
// sunset, read in their geocentric convention at geometric sunset. Ilyas
// offers it as a general guide only. Reads ArcL.
func (p *CrescentParams) Ilyas1983() bool {
	return p.ArcL >= 10.5
}

// ── Calendrical criteria: altitude and elongation at sunset ─────────────────

// MABIMS1995 evaluates the original MABIMS (1995) criterion used by Brunei,
// Indonesia, Malaysia, and Singapore for the Islamic calendar, the "2-3-8"
// criterion: MAlt ≥ 2°, AND ArcL ≥ 3° OR the Moon at least 8 hours old.
//
// MAlt is the Moon's topocentric altitude and ArcL its geocentric elongation,
// both at sunset. Superseded by MABIMS2021.
func (p *CrescentParams) MABIMS1995() bool {
	return p.MAlt >= 2.0 && (p.ArcL >= 3.0 || p.Age >= 8.0)
}

// MABIMS2021 evaluates the revised MABIMS (2021) criterion: ArcL ≥ 6.4° AND
// MAlt ≥ 3°.
//
// MAlt is the Moon's topocentric altitude and ArcL its geocentric elongation,
// both at sunset.
func (p *CrescentParams) MABIMS2021() bool {
	return p.ArcL >= 6.4 && p.MAlt >= 3.0
}

// Istanbul2016 evaluates the rule of the Istanbul congress of 2016 on a
// unified Hijri calendar: ArcL ≥ 8° AND MAlt ≥ 5° at sunset.
//
// The congress applies it anywhere on Earth before 24h UT, not at one site.
// The 8° and 5° are the Turkish Calendar Commission's of 1978 (Ilyas 1983,
// Fig. 2b). Neither source states whether the altitude and elongation are
// geocentric or topocentric, so they are read as MABIMS reads them:
// topocentric altitude, geocentric elongation.
func (p *CrescentParams) Istanbul2016() bool {
	return p.ArcL >= 8.0 && p.MAlt >= 5.0
}

// ── Arc of vision against the crescent's width ──────────────────────────────

// Bruin evaluates Bruin's (1977) criterion: ARCV ≥ 12.4023 − 9.4878W +
// 3.9512W² − 0.5632W³.
//
// Bruin, F. (1977), Vistas Astron. 21, 331, gave his criterion as curves
// (Fig. 9, p. 339); this cubic is Yallop's least-squares fit to them (1997,
// NAO TN 69, Table 3 and eq. 3.3), which do not extend beyond W = 3′. ARCV is
// geocentric, h + s; W = 15′(1 − cos ARCL), with Bruin's constant 15′
// semi-diameter and the geocentric ARCL (Yallop eq. 3.4). Bruin's optimum is
// the minimum of each curve, which Yallop's best time Ts + (4/9)·Lag
// reproduces (eq. 4.1). Reads ArcV and W.
func (p *CrescentParams) Bruin() bool {
	w := p.W

	return p.ArcV >= 12.4023-9.4878*w+3.9512*w*w-0.5632*w*w*w
}

// AlrefayNakedEye evaluates the naked-eye criterion of Alrefay et al. (2018):
// ARCV > 9.34 − 4.51W + 3.3W² − 1.01W³.
//
// Alrefay, T. et al. (2018), Observatory 138, 267, eq. 8, fitted to 545
// observations from Saudi Arabia, 1988–2015. W = SD(1 − cos ARCL) with SD
// held at 16′ (their eq. 7). They do not name the convention, but their
// Table I is topocentric: its arcs of vision at sunset match topocentric
// ones to 0.15° and miss geocentric ones by most of a degree
// (TestAlrefayTableIIsTopocentricAtSunset). Read at sunset. Reads ArcV and W.
func (p *CrescentParams) AlrefayNakedEye() bool {
	w := p.W

	return p.ArcV > 9.34-4.51*w+3.3*w*w-1.01*w*w*w
}

// AlrefayOpticalAid evaluates the aided-eye criterion of Alrefay et al.
// (2018), eq. 9: ARCV > 7.83 − 4.35W + 3.22W² − 1.02W³, in the convention of
// AlrefayNakedEye.
func (p *CrescentParams) AlrefayOpticalAid() bool {
	w := p.W

	return p.ArcV > 7.83-4.35*w+3.22*w*w-1.02*w*w*w
}

// Yallop evaluates the Yallop (1997) multi-zone criterion.
// It calculates the q parameter from ArcV and W, then classifies the
// observation into one of six visibility zones (A through F).
//
// The q parameter is:
//
//	q = (ArcV − 11.8371 + 6.3226·W − 0.7319·W² + 0.1018·W³) / 10
//
// Zones:
//
//	A: Easily visible               (q > +0.216)
//	B: Visible under perfect cond.  (+0.216 ≥ q > −0.014)
//	C: May need optical aid         (−0.014 ≥ q > −0.160)
//	D: Will need optical aid        (−0.160 ≥ q > −0.232)
//	E: Not visible with telescope   (−0.232 ≥ q > −0.293)
//	F: Not visible, below Danjon    (−0.293 ≥ q)
//
// Yallop defined ArcV as geocentric and airless, and W as his W′ from the
// geocentric elongation, at his best time (NAO TN 69, 1997).
func (p *CrescentParams) Yallop() CrescentZone {
	w := p.W
	q := (p.ArcV - 11.8371 + 6.3226*w - 0.7319*w*w + 0.1018*w*w*w) / 10.0

	switch {
	case q > 0.216:
		return CrescentZone{Code: "A", Label: "Easily visible", Value: q, Params: *p}
	case q > -0.014:
		return CrescentZone{Code: "B", Label: "Visible under perfect conditions", Value: q, Params: *p}
	case q > -0.160:
		return CrescentZone{Code: "C", Label: "May need optical aid", Value: q, Params: *p}
	case q > -0.232:
		return CrescentZone{Code: "D", Label: "Will need optical aid", Value: q, Params: *p}
	case q > -0.293:
		return CrescentZone{Code: "E", Label: "Not visible with telescope", Value: q, Params: *p}
	default:
		return CrescentZone{Code: "F", Label: "Not visible, below Danjon limit", Value: q, Params: *p}
	}
}

// Odeh evaluates the Odeh (2004) multi-zone criterion.
// It calculates the V parameter from ArcV and W, then classifies the
// observation into one of four visibility zones.
//
// The V parameter is:
//
//	V = ArcV − (−0.1018·W³ + 0.7319·W² − 6.3226·W + 7.1651)
//
// Zones:
//
//	Naked Eye                  (V ≥ 5.65)
//	Optical Aid / Maybe Naked  (5.65 > V ≥ 2.0)
//	Optical Aid Only           (2.0 > V ≥ −0.96)
//	Not Visible                (V < −0.96)
//
// Odeh's ArcV and W are topocentric and airless, at Yallop's best time
// (Exp. Astron. 18, 39, 2004).
func (p *CrescentParams) Odeh() CrescentZone {
	w := p.W
	v := p.ArcV - (-0.1018*w*w*w + 0.7319*w*w - 6.3226*w + 7.1651)

	switch {
	case v >= 5.65:
		return CrescentZone{Code: "Naked Eye", Label: "Visible to naked eye", Value: v, Params: *p}
	case v >= 2.0:
		return CrescentZone{Code: "Optical/Naked", Label: "Optical aid, may be seen by naked eye", Value: v, Params: *p}
	case v >= -0.96:
		return CrescentZone{Code: "Optical Only", Label: "Visible only with optical aid", Value: v, Params: *p}
	default:
		return CrescentZone{Code: "Not Visible", Label: "Not visible", Value: v, Params: *p}
	}
}

// Qureshi evaluates the Qureshi (2010) multi-zone criterion:
//
//	s = (ArcV − (10.43418 − 5.422643·W + 2.222075·W² − 0.351964·W³)) / 10
//
// Qureshi, M. S. (2010), Sindh Univ. Res. J. (Sci. Ser.) 42(1), 1, eq. 4 and
// Table 6, with W in arcminutes. His eq. 5 as printed drops the parentheses
// around the cubic, and read literally adds it to ArcV, putting every
// crescent in zone A; his own Table 5 follows eq. 4 (obs. 220: ARCV 6.03°,
// W 23.3″, s = −0.26). That table uses Yallop's quantities, geocentric ArcV
// and topocentric W′, which Qureshi evaluates at his own best time,
// Ts + (4.3/9.3)·Lag (eq. 6).
//
// Zones:
//
//	A: Easily visible                (s > 0.15)
//	B: Visible under perfect cond.   (0.15 ≥ s > 0.05)
//	C: May require optical aid       (0.05 ≥ s > −0.06)
//	D: Require optical aid           (−0.06 ≥ s > −0.16)
//	E: Not visible with optical aid  (s ≤ −0.16)
func (p *CrescentParams) Qureshi() CrescentZone {
	w := p.W
	s := (p.ArcV - (10.43418 - 5.422643*w + 2.222075*w*w - 0.351964*w*w*w)) / 10.0

	switch {
	case s > 0.15:
		return CrescentZone{Code: "A", Label: "Easily visible", Value: s, Params: *p}
	case s > 0.05:
		return CrescentZone{Code: "B", Label: "Visible under perfect conditions", Value: s, Params: *p}
	case s > -0.06:
		return CrescentZone{Code: "C", Label: "May require optical aid", Value: s, Params: *p}
	case s > -0.16:
		return CrescentZone{Code: "D", Label: "Require optical aid", Value: s, Params: *p}
	default:
		return CrescentZone{Code: "E", Label: "Not visible with optical aid", Value: s, Params: *p}
	}
}

// ── The SAAO criterion: apparent altitude against DAZ ───────────────────────

// caldwell2001NakedEye is the upper line of Caldwell & Laney (2001), Table 1
// (Fig. 1): the apparent altitude of the Moon's lower limb at sunset below
// which a naked-eye sighting is improbable, every 0.5° of DAZ from 0° to 21°.
// Their lower line, below which even an aided sighting is impossible, is
// this one less 1.90° throughout.
var caldwell2001NakedEye = []float64{
	8.19, 8.18, 8.16, 8.14, 8.10, 8.06, 8.02, 7.96, 7.91, 7.84, 7.77, 7.70, 7.62, 7.53, 7.44,
	7.35, 7.26, 7.16, 7.05, 6.95, 6.84, 6.73, 6.61, 6.50, 6.38, 6.26, 6.15, 6.03, 5.91, 5.79,
	5.67, 5.55, 5.43, 5.31, 5.19, 5.08, 4.96, 4.85, 4.74, 4.64, 4.53, 4.43, 4.33,
}

// caldwell2001Aided is the lower line of the same table.
var caldwell2001Aided = []float64{
	6.29, 6.28, 6.26, 6.24, 6.20, 6.16, 6.12, 6.06, 6.01, 5.94, 5.87, 5.80, 5.72, 5.63, 5.54,
	5.45, 5.36, 5.26, 5.15, 5.05, 4.94, 4.83, 4.71, 4.60, 4.48, 4.36, 4.25, 4.13, 4.01, 3.89,
	3.77, 3.65, 3.53, 3.41, 3.29, 3.18, 3.06, 2.95, 2.84, 2.74, 2.63, 2.53, 2.43,
}

// CaldwellNakedEye evaluates the SAAO criterion of Caldwell & Laney (2001)
// for the naked eye: the Moon's lower limb must reach their upper line.
//
// Caldwell, J. A. R. & Laney, C. D. (2001), African Skies 5, 15, Table 1:
// the apparent altitude of the Moon's lower limb, topocentric and refracted,
// at sunset, against DAZ at sunset. MAlt is that apparent lower-limb
// altitude. Their lines are drawn to the edge of reliable sightings and are
// meant to be optimistic. Beyond DAZ 21°, the end of the table, its last
// value holds. Reads MAlt and DAZ.
func (p *CrescentParams) CaldwellNakedEye() bool {
	return p.MAlt >= interpolateUniform(caldwell2001NakedEye, 0.5, math.Abs(p.DAZ))
}

// CaldwellOptical evaluates the SAAO criterion of Caldwell & Laney (2001)
// with optical aid: the Moon's lower limb must reach their lower line, in
// the convention of CaldwellNakedEye.
func (p *CrescentParams) CaldwellOptical() bool {
	return p.MAlt >= interpolateUniform(caldwell2001Aided, 0.5, math.Abs(p.DAZ))
}

// ── Tables ──────────────────────────────────────────────────────────────────

// interpolateUniform interpolates linearly in ys, tabulated every step from
// 0, holding the last value beyond the table.
func interpolateUniform(ys []float64, step, x float64) float64 {
	if x <= 0 {
		return ys[0]
	}

	f := x / step

	i := int(f)
	if i >= len(ys)-1 {
		return ys[len(ys)-1]
	}

	return ys[i] + (f-float64(i))*(ys[i+1]-ys[i])
}

// interpolateTable interpolates linearly in ys against the ascending xs,
// holding the end values beyond the table.
func interpolateTable(xs, ys []float64, x float64) float64 {
	if x <= xs[0] {
		return ys[0]
	}

	for i := 1; i < len(xs); i++ {
		if x <= xs[i] {
			return ys[i-1] + (x-xs[i-1])/(xs[i]-xs[i-1])*(ys[i]-ys[i-1])
		}
	}

	return ys[len(ys)-1]
}

// ── The evening's result ────────────────────────────────────────────────────

// CrescentResult is CrescentVisibility's evaluation of one evening: its
// instants, and every criterion's answer with the quantities it read.
type CrescentResult struct {
	// Sunset and Moonset are the evening's sunset and the moonset its lag
	// runs to; GeometricSunset is when the Sun's center reached altitude 0,
	// the "sunset" of the azimuth–altitude criteria.
	Sunset, Moonset, GeometricSunset time.Time

	// BestTime is Yallop's best time, Ts + (4/9)·Lag, at which Bruin, Yallop
	// and Odeh are read; QureshiBestTime is Qureshi's, Ts + (4.3/9.3)·Lag.
	BestTime, QureshiBestTime time.Time

	Yallop  CrescentZone
	Odeh    CrescentZone
	Qureshi CrescentZone

	Fotheringham   CrescentVerdict
	Maunder        CrescentVerdict
	Ilyas1988      CrescentVerdict
	KraussAthenian CrescentVerdict

	Danjon      CrescentVerdict
	Fatoohi1998 CrescentVerdict
	Ilyas1983   CrescentVerdict

	MABIMS1995   CrescentVerdict
	MABIMS2021   CrescentVerdict
	Istanbul2016 CrescentVerdict

	Bruin             CrescentVerdict
	AlrefayNakedEye   CrescentVerdict
	AlrefayOpticalAid CrescentVerdict

	CaldwellNakedEye CrescentVerdict
	CaldwellOptical  CrescentVerdict
}

// String returns a multi-line summary: the evening's instants, then every
// criterion's answer with the quantities it read.
func (r CrescentResult) String() string {
	var b strings.Builder

	fmt.Fprintf(&b, "Lunar crescent visibility\n")
	fmt.Fprintf(&b, "  sunset %v, moonset %v, best time %v\n", r.Sunset, r.Moonset, r.BestTime)

	verdict := func(name string, v CrescentVerdict) {
		yn := "not visible"
		if v.Visible {
			yn = "visible"
		}

		fmt.Fprintf(&b, "  %-22s %-12s ARCV %6.2f° ARCL %6.2f° DAZ %6.2f° alt %6.2f° W %5.2f′\n",
			name, yn, v.Params.ArcV, v.Params.ArcL, v.Params.DAZ, v.Params.MAlt, v.Params.W)
	}

	zone := func(name string, z CrescentZone) {
		fmt.Fprintf(&b, "  %-22s %-12s ARCV %6.2f° W %5.2f′  %s\n",
			name, z.Code, z.Params.ArcV, z.Params.W, z.String())
	}

	verdict("Fotheringham (1910)", r.Fotheringham)
	verdict("Maunder (1911)", r.Maunder)
	verdict("Ilyas (1988)", r.Ilyas1988)
	verdict("Krauss Athens (2012)", r.KraussAthenian)
	verdict("Danjon (1932)", r.Danjon)
	verdict("Fatoohi (1998)", r.Fatoohi1998)
	verdict("Ilyas (1983)", r.Ilyas1983)
	verdict("MABIMS (1995)", r.MABIMS1995)
	verdict("MABIMS (2021)", r.MABIMS2021)
	verdict("Istanbul (2016)", r.Istanbul2016)
	verdict("Bruin (1977)", r.Bruin)
	verdict("Alrefay naked (2018)", r.AlrefayNakedEye)
	verdict("Alrefay aided (2018)", r.AlrefayOpticalAid)
	verdict("SAAO naked (2001)", r.CaldwellNakedEye)
	verdict("SAAO aided (2001)", r.CaldwellOptical)
	zone("Yallop (1997)", r.Yallop)
	zone("Odeh (2004)", r.Odeh)
	zone("Qureshi (2010)", r.Qureshi)

	return strings.TrimSuffix(b.String(), "\n")
}
