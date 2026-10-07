package plan

import (
	"fmt"
	"math"
	"slices"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// meanSolarLongitudeDegPerDay is the Sun's mean apparent motion along the
// ecliptic, 360°/365.25 days. RadiantAt uses it to turn the Sun's real
// longitude offset from the peak into the day count the radiant's daily
// drift is multiplied by. The activity windows do not use it: in July and
// August the Sun moves 0.957°/day, so a day count converted at the mean rate
// put the Perseids' start most of a day before IMO's date (#573).
const meanSolarLongitudeDegPerDay = 360.0 / 365.25

// MeteorShower describes one annual meteor shower's radiant motion and
// activity profile, following the IMO (International Meteor Organization)
// working-list parameterization: the radiant position is anchored to a
// peak SOLAR LONGITUDE, not a calendar date — solar longitude is exact
// and year-independent (a calendar date drifts by up to a day year to
// year).
//
// IMO gives every solar longitude "for the equinox 2000.0", so that is
// what they are compared with (sunLongitudeJ2000). Until #415 they were
// compared with the Sun's longitude of date, 0.37° further on in 2026 by
// general precession, and every window and peak came about 9 hours early,
// drifting 20 minutes further each year.
type MeteorShower struct {
	// Name is the shower's common name (e.g. "Perseids").
	Name string
	// Code is the IAU 3-letter shower code (e.g. "PER").
	Code string
	// ParentBody is the comet or asteroid this shower's meteoroid stream
	// originates from — informational only, not used in any computation.
	ParentBody string

	// RadiantRA/RadiantDec are the radiant's position AT PeakSolarLongitude.
	RadiantRA, RadiantDec angle.Angle
	// DriftRAPerDay/DriftDecPerDay are the radiant's daily motion near
	// peak, in degrees/day (as an Angle purely for unit consistency —
	// there is no "per day" angle type; RadiantAt multiplies this by an
	// elapsed-day count derived from the actual solar-longitude
	// difference, not a calendar-day count).
	DriftRAPerDay, DriftDecPerDay angle.Angle

	// ActiveStartSolarLon/ActiveEndSolarLon/PeakSolarLongitude are all in
	// degrees of solar longitude (0-360). The shower is considered active
	// while the Sun's real ecliptic longitude of date falls within
	// [ActiveStartSolarLon, ActiveEndSolarLon] — IsActive/solarLongitudeInRange
	// handle the window wrapping past 360°→0° should a shower's dates
	// require it (none of the starter table's do, but the logic doesn't
	// assume that).
	ActiveStartSolarLon, ActiveEndSolarLon, PeakSolarLongitude float64

	// ZHR is the Zenithal Hourly Rate: meteors/hour a single observer
	// would see under ideal conditions (radiant at zenith, limiting
	// magnitude 6.5). See ObservedRate for the real-conditions formula.
	ZHR float64
	// Activity is how the ZHR falls away from its maximum; see
	// [ActivityProfile]. The zero value is flat, the ZHR staying at its
	// maximum through the whole activity window, which is all that can be
	// said of a shower whose profile is not known.
	Activity ActivityProfile
	// PopulationIndex (r) describes how the shower's meteor count changes
	// per magnitude of limiting-magnitude depth — always > 1; most
	// showers fall in the 2.0-3.2 range (lower = relatively more bright
	// meteors).
	PopulationIndex float64
	// Velocity is the shower's geocentric entry velocity, informational.
	Velocity unit.Velocity
}

// ActivityProfile is a shower's activity around its maximum, as a fraction
// of the activity at the maximum, in the form Jenniskens (1994, A&A 287,
// 990) fits to the annual streams: the ZHR falls off as
//
//	10^(−B·|λ☉ − λ☉max|)
//
// with the slope B per degree of solar longitude (his Eq. 8). A stream that
// one such curve does not describe is the sum of two, a narrow main peak and
// a broad background (his Table 3c). The rising branch, before the maximum,
// has a slope B⁺ of its own, and the falling branch B⁻.
//
// PeakShare is in [0, 1] and every slope is non-negative. The zero value is
// flat: no peak, and a background that does not fall off.
type ActivityProfile struct {
	// PeakShare is the main peak's share of the activity at the maximum,
	// ZHRᵖ / (ZHRᵖ + ZHRᵇ); the background has the rest. It is 1 for a
	// stream one curve describes.
	PeakShare float64

	// PeakRise and PeakFall are the main peak's slopes B⁺ and B⁻, per
	// degree of solar longitude.
	PeakRise, PeakFall float64

	// BackgroundRise and BackgroundFall are the background's.
	BackgroundRise, BackgroundFall float64
}

// at returns the activity delta degrees of solar longitude from the
// maximum, negative before it, as a fraction of the activity at the maximum.
func (p ActivityProfile) at(delta float64) float64 {
	peak, background := p.PeakFall, p.BackgroundFall
	if delta < 0 {
		peak, background = p.PeakRise, p.BackgroundRise
	}

	d := math.Abs(delta)

	return p.PeakShare*math.Pow(10, -peak*d) + (1-p.PeakShare)*math.Pow(10, -background*d)
}

// MeteorShowerNames returns the names of every built-in shower, sorted.
//
// The table behind it is unexported, for the reason [KnownSiteNames] gives.
// Use [NewMeteorShower] to resolve a name.
func MeteorShowerNames() []string {
	out := make([]string, 0, len(meteorShowers))

	for _, s := range meteorShowers {
		out = append(out, s.Name)
	}

	slices.Sort(out)

	return out
}

// meteorShowers is a modest, defensible starter list, not the full IMO
// working list: the 9 IMO "Class I" (strongest annual) showers, keyed by a
// lowercase/underscore slug. See NewMeteorShower for name/code-based lookup.
//
// Every value is from the IMO Meteor Shower Calendar 2027
// (https://www.imo.net/ShCal27s.pdf), "correct according to the best
// information available in June 2026" (#573):
//
//   - Peak solar longitude, radiant at the peak, V∞, r and ZHR are Table 5's,
//     with its decimals. Where Table 5 gives a ZHR as a floor ("80+", "110+",
//     "15+") the floor is used: the Quadrantids "can vary ≈ 60 − 200", and the
//     Leonids' storms, tied to 55P/Tempel-Tuttle's 33-year period, are
//     nothing this model can predict.
//   - The daily drift is the difference of the two Table 6 radiant positions
//     that bracket the maximum, divided by the days between them. Table 6
//     publishes positions to the whole degree five days apart, so a drift is
//     good to about 0.2°/day.
//   - The activity window is Table 5's dates turned into the Sun's J2000
//     longitude, geometric, on ecliptic_J2000_frame (Skyfield 1.55, DE440s):
//     0h UT on the first date of activity to 24h UT on the last, in 2027, the
//     year the calendar is for.
//
// Each activity profile is Jenniskens (1994, A&A 287, 990)'s, centered on
// IMO's maximum (his are for the equinox 1950.0). Where his Table 3c fits a
// main peak and a background, that is used: the Quadrantids, Perseids,
// Geminids and Ursids, whose long backgrounds one curve cannot follow. The
// rest are his Table 3b's single curves, the Leonids among them because
// Table 3c gives their falling background only as a bound, "> 0.15". The
// Ursids' Table 3b fit is parenthesized there as uncertain; Table 3c's is
// not. The ZHR at the maximum stays IMO's: only the shape is his.
//
// The Ursids' radiant, at +76°, is close enough to the pole that a drift in
// right ascension is near-degenerate; Table 6 gives 217° on both dates that
// bracket the maximum, so its RA drift is zero and its declination drift is
// Table 6's.
var meteorShowers = map[string]MeteorShower{
	"quadrantids": {
		Name: "Quadrantids", Code: "QUA", ParentBody: "2003 EH1",
		RadiantRA: angle.Deg(230), RadiantDec: angle.Deg(49),
		DriftRAPerDay: angle.Deg(0.6), DriftDecPerDay: angle.Deg(-0.2),
		PeakSolarLongitude: 283.15, ActiveStartSolarLon: 275.87, ActiveEndSolarLon: 292.18,
		ZHR: 80, PopulationIndex: 2.1, Velocity: unit.KmPerSec(41),
		Activity: ActivityProfile{PeakShare: 110.0 / (110 + 20), PeakRise: 2.5, PeakFall: 2.5, BackgroundRise: 0.37, BackgroundFall: 0.45},
	},
	"lyrids": {
		Name: "Lyrids", Code: "LYR", ParentBody: "C/1861 G1 (Thatcher)",
		RadiantRA: angle.Deg(271), RadiantDec: angle.Deg(34),
		DriftRAPerDay: angle.Deg(1.0), DriftDecPerDay: angle.Deg(0.0),
		PeakSolarLongitude: 32.32, ActiveStartSolarLon: 23.46, ActiveEndSolarLon: 40.04,
		ZHR: 18, PopulationIndex: 2.1, Velocity: unit.KmPerSec(49),
		Activity: ActivityProfile{PeakShare: 1, PeakRise: 0.22, PeakFall: 0.22},
	},
	"eta_aquariids": {
		Name: "Eta Aquariids", Code: "ETA", ParentBody: "1P/Halley",
		RadiantRA: angle.Deg(338), RadiantDec: angle.Deg(-1),
		DriftRAPerDay: angle.Deg(0.8), DriftDecPerDay: angle.Deg(0.4),
		PeakSolarLongitude: 45.5, ActiveStartSolarLon: 28.35, ActiveEndSolarLon: 67.05,
		ZHR: 50, PopulationIndex: 2.4, Velocity: unit.KmPerSec(66),
		Activity: ActivityProfile{PeakShare: 1, PeakRise: 0.080, PeakFall: 0.080},
	},
	"southern_delta_aquariids": {
		Name: "Southern Delta Aquariids", Code: "SDA", ParentBody: "96P/Machholz (disputed)",
		RadiantRA: angle.Deg(340), RadiantDec: angle.Deg(-16),
		DriftRAPerDay: angle.Deg(0.83), DriftDecPerDay: angle.Deg(0.33),
		PeakSolarLongitude: 128, ActiveStartSolarLon: 109.08, ActiveEndSolarLon: 150.25,
		ZHR: 25, PopulationIndex: 2.5, Velocity: unit.KmPerSec(41),
		Activity: ActivityProfile{PeakShare: 1, PeakRise: 0.091, PeakFall: 0.091},
	},
	"perseids": {
		Name: "Perseids", Code: "PER", ParentBody: "109P/Swift-Tuttle",
		RadiantRA: angle.Deg(48), RadiantDec: angle.Deg(58),
		DriftRAPerDay: angle.Deg(1.2), DriftDecPerDay: angle.Deg(0.2),
		PeakSolarLongitude: 140.0, ActiveStartSolarLon: 113.85, ActiveEndSolarLon: 151.21,
		ZHR: 110, PopulationIndex: 2.2, Velocity: unit.KmPerSec(59),
		Activity: ActivityProfile{PeakShare: 70.0 / (70 + 23), PeakRise: 0.35, PeakFall: 0.35, BackgroundRise: 0.050, BackgroundFall: 0.092},
	},
	"orionids": {
		Name: "Orionids", Code: "ORI", ParentBody: "1P/Halley",
		RadiantRA: angle.Deg(95), RadiantDec: angle.Deg(16),
		DriftRAPerDay: angle.Deg(0.8), DriftDecPerDay: angle.Deg(0.0),
		PeakSolarLongitude: 208, ActiveStartSolarLon: 188.20, ActiveEndSolarLon: 224.96,
		ZHR: 20, PopulationIndex: 2.5, Velocity: unit.KmPerSec(66),
		Activity: ActivityProfile{PeakShare: 1, PeakRise: 0.12, PeakFall: 0.12},
	},
	"leonids": {
		Name: "Leonids", Code: "LEO", ParentBody: "55P/Tempel-Tuttle",
		RadiantRA: angle.Deg(152), RadiantDec: angle.Deg(22),
		DriftRAPerDay: angle.Deg(0.6), DriftDecPerDay: angle.Deg(-0.4),
		PeakSolarLongitude: 235.27, ActiveStartSolarLon: 222.96, ActiveEndSolarLon: 248.16,
		ZHR: 15, PopulationIndex: 2.5, Velocity: unit.KmPerSec(71),
		Activity: ActivityProfile{PeakShare: 1, PeakRise: 0.39, PeakFall: 0.39},
	},
	"geminids": {
		Name: "Geminids", Code: "GEM", ParentBody: "3200 Phaethon",
		RadiantRA: angle.Deg(112), RadiantDec: angle.Deg(33),
		DriftRAPerDay: angle.Deg(1.0), DriftDecPerDay: angle.Deg(0.0),
		PeakSolarLongitude: 262.2, ActiveStartSolarLon: 251.20, ActiveEndSolarLon: 268.48,
		ZHR: 150, PopulationIndex: 2.6, Velocity: unit.KmPerSec(35),
		Activity: ActivityProfile{PeakShare: 74.0 / (74 + 18), PeakRise: 0.59, PeakFall: 0.81, BackgroundRise: 0.09, BackgroundFall: 0.31},
	},
	"ursids": {
		Name: "Ursids", Code: "URS", ParentBody: "8P/Tuttle",
		RadiantRA: angle.Deg(217), RadiantDec: angle.Deg(76),
		DriftRAPerDay: angle.Zero(), DriftDecPerDay: angle.Deg(-0.4),
		PeakSolarLongitude: 270.7, ActiveStartSolarLon: 264.41, ActiveEndSolarLon: 274.59,
		ZHR: 10, PopulationIndex: 2.8, Velocity: unit.KmPerSec(33),
		Activity: ActivityProfile{PeakShare: 10.0 / (10 + 2.0), PeakRise: 0.9, PeakFall: 0.9, BackgroundRise: 0.08, BackgroundFall: 0.2},
	},
}

// NewMeteorShower looks up name against each built-in shower's Name or Code
// ([MeteorShowerNames] lists them), case- and space-insensitive, and returns
// it, or ErrUnknownMeteorShower if no entry matches.
func NewMeteorShower(name string) (MeteorShower, error) {
	want := normalizeSiteName(name)

	if m, ok := meteorShowers[want]; ok {
		return m, nil
	}

	for _, m := range meteorShowers {
		if normalizeSiteName(m.Code) == want {
			return m, nil
		}
	}

	return MeteorShower{}, fmt.Errorf("%w: %q", ErrUnknownMeteorShower, name)
}

// solarLongitudeDelta returns cur-peak wrapped to (-180, 180] degrees —
// the shortest signed angular distance from peak to cur, correctly
// handling the case where the shower's peak sits near the 0°/360° solar-
// longitude boundary (e.g. a date just after New Year for a peak in late
// December).
func solarLongitudeDelta(cur, peak float64) float64 {
	d := cur - peak
	for d > 180 {
		d -= 360
	}

	for d <= -180 {
		d += 360
	}

	return d
}

// solarLongitudeInRange reports whether lambda falls within [start, end]
// (degrees, each wrapped to [0,360)), handling the case where the range
// itself wraps past 360°→0°.
//
// A range a full turn or more wide contains every longitude. Wrapping its
// ends first made [0, 360] the single point 0, so a shower declared active
// all year was active at one instant of it (#572).
func solarLongitudeInRange(lambda, start, end float64) bool {
	if end-start >= 360 {
		return true
	}

	lambda = wrap360Deg(lambda)
	start = wrap360Deg(start)
	end = wrap360Deg(end)

	if start <= end {
		return lambda >= start && lambda <= end
	}

	return lambda >= start || lambda <= end
}

func wrap360Deg(d float64) float64 {
	d = math.Mod(d, 360)
	if d < 0 {
		d += 360
	}

	return d
}

// IsActive reports whether m is active at time t, from the Sun's longitude
// referred to the equinox J2000.0, as IMO tabulates its activity windows —
// not a calendar-date range.
func (m MeteorShower) IsActive(t time.Time, prov eph.Provider) (bool, error) {
	lambda, err := sunLongitudeJ2000(t, prov)
	if err != nil {
		return false, fmt.Errorf("meteor: active: %w", err)
	}

	return solarLongitudeInRange(lambda, m.ActiveStartSolarLon, m.ActiveEndSolarLon), nil
}

// RadiantAt returns m's radiant position at time t: the Sun's longitude,
// referred to the equinox J2000.0 as IMO's are, is compared against
// m.PeakSolarLongitude, converted to an elapsed-day count via
// meanSolarLongitudeDegPerDay, and applied as linear RA/Dec drift from the
// peak position.
func (m MeteorShower) RadiantAt(t time.Time, prov eph.Provider) (ra, dec angle.Angle, err error) {
	lambda, err := sunLongitudeJ2000(t, prov)
	if err != nil {
		return angle.Zero(), angle.Zero(), fmt.Errorf("meteor: radiant: %w", err)
	}

	deltaDays := solarLongitudeDelta(lambda, m.PeakSolarLongitude) / meanSolarLongitudeDegPerDay

	ra = m.RadiantRA.Add(m.DriftRAPerDay.MulScalar(deltaDays)).Wrap2Pi()
	dec = m.RadiantDec.Add(m.DriftDecPerDay.MulScalar(deltaDays))

	return ra, dec, nil
}

// Radiant returns m's radiant at time t as a *Star — a radiant is just a
// fixed sky point at a given moment, and Star already implements exactly
// that; no bespoke Observable type is needed for a meteor shower itself.
func (m MeteorShower) Radiant(t time.Time, prov eph.Provider) (*Star, error) {
	ra, dec, err := m.RadiantAt(t, prov)
	if err != nil {
		return nil, err
	}

	return NewStar(m.Name+" radiant", ra, dec), nil
}

// ZHRAt returns m's zenithal hourly rate at time t: m.ZHR, the rate at the
// maximum, times m.Activity at the Sun's J2000 longitude, and zero outside
// [m.ActiveStartSolarLon, m.ActiveEndSolarLon].
//
// Until #572 nothing used the date: ObservedRate gave the maximum rate on
// every night of the year, 97 Perseids an hour on 1 March from 45°N.
func (m MeteorShower) ZHRAt(t time.Time, prov eph.Provider) (float64, error) {
	lambda, err := sunLongitudeJ2000(t, prov)
	if err != nil {
		return 0, fmt.Errorf("meteor: ZHR: %w", err)
	}

	if !solarLongitudeInRange(lambda, m.ActiveStartSolarLon, m.ActiveEndSolarLon) {
		return 0, nil
	}

	return m.ZHR * m.Activity.at(solarLongitudeDelta(lambda, m.PeakSolarLongitude)), nil
}

// ObservedRate returns the predicted number of m's meteors a single
// observer at site would see per hour at time t, given the naked-eye
// limiting magnitude limitingMag actually reached under the sky
// conditions of the moment. This is IMO's own standard formula, inverted
// to predict rather than measure:
//
//	observedRate = ZHR(t) · sin(h_R) · r^(LM − 6.5)
//
// where ZHR(t) is [MeteorShower.ZHRAt] and h_R is the radiant's altitude.
// Under the defining standard conditions (h_R=90°, LM=6.5) it reduces to
// exactly ZHR(t). Returns 0 (not an error) outside the activity window,
// when the radiant is below the horizon, and for a limiting magnitude of
// -Inf (a sky too bright to see anything).
//
// The limiting magnitude is a caller-supplied input rather than something
// computed here: it depends on moonlight, zodiacal light and light
// pollution through a sky-brightness model, which is a separate concern
// from meteor rate arithmetic.
func (m MeteorShower) ObservedRate(t time.Time, site *Site, prov eph.Provider, limitingMag float64) (float64, error) {
	zhr, err := m.ZHRAt(t, prov)
	if err != nil {
		return 0, fmt.Errorf("meteor: observed rate: %w", err)
	}

	if zhr == 0 {
		return 0, nil
	}

	ra, dec, err := m.RadiantAt(t, prov)
	if err != nil {
		return 0, fmt.Errorf("meteor: observed rate: %w", err)
	}

	ctx := coord.NewContext(t, site.Location(), site.Refraction())

	aa, err := ctx.ICRSToAltAz(coord.NewICRS(ra, dec))
	if err != nil {
		return 0, fmt.Errorf("meteor: observed rate: %w", err)
	}

	if aa.Alt().Degrees() <= 0 {
		return 0, nil
	}

	if math.IsInf(limitingMag, -1) {
		return 0, nil
	}

	return zhr * aa.Alt().Sin() * math.Pow(m.PopulationIndex, limitingMag-6.5), nil
}

// sunLongitudeJ2000 is the Sun's geocentric ecliptic longitude referred to the
// mean equinox and ecliptic of J2000.0, in degrees [0, 360): the solar
// longitude meteor-shower catalogs tabulate (IMO: "All λ⊙ are given for the
// equinox 2000.0").
//
// The geometric Sun, rotated from ICRS about x by the J2000 obliquity: no
// precession, nutation or aberration. The frame bias between ICRS and the
// J2000 equator, 0.02″, is far below anything a shower's timing resolves.
// Against Skyfield's ecliptic_J2000_frame (DE421) it agrees to a thousandth
// of a degree, about a minute of time.
func sunLongitudeJ2000(t time.Time, prov eph.Provider) (float64, error) {
	sun, err := eph.Position(prov, eph.Sun, t)
	if err != nil {
		return 0, fmt.Errorf("meteor: sun position: %w", err)
	}

	sinEps, cosEps := math.Sincos(constants.IAU.ObliquityJ2000.Value)

	lon := math.Atan2(sun.Y*cosEps+sun.Z*sinEps, sun.X) * 180 / math.Pi
	if lon < 0 {
		lon += 360
	}

	return lon, nil
}
