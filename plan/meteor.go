package plan

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// meanSolarLongitudeDegPerDay is the Sun's mean apparent motion along the
// ecliptic, 360°/365.25 days — used only to translate an IMO calendar-date
// activity window into an approximate solar-longitude window (see
// meteorShowers' doc comment); RadiantAt itself always uses the Sun's real
// computed ecliptic longitude, never this mean rate.
const meanSolarLongitudeDegPerDay = 360.0 / 365.25

// MeteorShower describes one annual meteor shower's radiant motion and
// activity profile, following the IMO (International Meteor Organization)
// working-list parameterization: the radiant position is anchored to a
// peak SOLAR LONGITUDE, not a calendar date — solar longitude is exact
// and year-independent (a calendar date drifts by up to a day year to
// year), and this codebase already has the primitive needed to compute
// it precisely (the unexported sunEclipticLongitude, used by Seasons'
// equinox/solstice solver).
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
	// PopulationIndex (r) describes how the shower's meteor count changes
	// per magnitude of limiting-magnitude depth — always > 1; most
	// showers fall in the 2.0-3.2 range (lower = relatively more bright
	// meteors).
	PopulationIndex float64
	// VelocityKmS is the shower's geocentric entry velocity, informational.
	VelocityKmS float64
}

// MeteorShowers is a modest, defensible starter list, not the full IMO
// working list — the 9 IMO "Class I" (strongest annual) showers, keyed by
// a lowercase/underscore slug. See NewMeteorShower for name/code-based
// lookup. Peak
// radiant position (RA/Dec at PeakSolarLongitude), ZHR, population index,
// and velocity are IMO's own published Table 5 values (2015/2020-era IMO
// Meteor Shower Calendar, imo.net); daily drift near each peak is derived
// from IMO's Table 6 (radiant position at bracketing dates around each
// peak, differenced here rather than copied as a pre-computed rate, since
// the source table publishes positions, not rates); each ActiveStartSolarLon/
// ActiveEndSolarLon is approximated from IMO's published calendar-date
// activity window via meanSolarLongitudeDegPerDay, since the source
// doesn't publish the window's solar-longitude bounds directly — an
// approximation, not independently sourced, documented here rather than
// silently treated as exact.
//
// Leonids' ZHR is set to its typical annual (non-outburst) rate, not the
// "100+" IMO's own table lists — that figure reflects the shower's famous
// ~33-year storm potential (tied to 55P/Tempel-Tuttle's orbital period),
// which this simple model has no mechanism to predict; using it as a
// blanket annual ZHR would badly overstate every ordinary year.
//
// Ursids' daily drift is left at zero: the radiant sits at Dec +76°, close
// enough to the north celestial pole that "degrees of RA per day" becomes
// an unstable, near-degenerate quantity (the same phenomenon documented on
// constellation.Centroid for Ursa Minor) — the source data available
// wasn't sufficient to derive a reliable rate, and near the peak date the
// resulting position error from omitting it is small in absolute terms.
//
//nolint:gochecknoglobals // fixed reference data, same convention as plan.PlanetaryMoons/KnownSites
var MeteorShowers = map[string]MeteorShower{
	"quadrantids": {
		Name: "Quadrantids", Code: "QUA", ParentBody: "2003 EH1",
		RadiantRA: angle.Deg(230), RadiantDec: angle.Deg(49),
		DriftRAPerDay: angle.Deg(0.6), DriftDecPerDay: angle.Deg(-0.2),
		PeakSolarLongitude: 283, ActiveStartSolarLon: 283 - 2*meanSolarLongitudeDegPerDay, ActiveEndSolarLon: 283 + 2*meanSolarLongitudeDegPerDay,
		ZHR: 120, PopulationIndex: 2.1, VelocityKmS: 41,
	},
	"lyrids": {
		Name: "Lyrids", Code: "LYR", ParentBody: "C/1861 G1 (Thatcher)",
		RadiantRA: angle.Deg(271), RadiantDec: angle.Deg(34),
		DriftRAPerDay: angle.Deg(1.0), DriftDecPerDay: angle.Deg(0.0),
		PeakSolarLongitude: 32, ActiveStartSolarLon: 32 - 6*meanSolarLongitudeDegPerDay, ActiveEndSolarLon: 32 + 3*meanSolarLongitudeDegPerDay,
		ZHR: 18, PopulationIndex: 2.1, VelocityKmS: 49,
	},
	"eta_aquariids": {
		Name: "Eta Aquariids", Code: "ETA", ParentBody: "1P/Halley",
		RadiantRA: angle.Deg(338), RadiantDec: angle.Deg(-1),
		DriftRAPerDay: angle.Deg(0.8), DriftDecPerDay: angle.Deg(0.4),
		PeakSolarLongitude: 45, ActiveStartSolarLon: 45 - 17*meanSolarLongitudeDegPerDay, ActiveEndSolarLon: 45 + 22*meanSolarLongitudeDegPerDay,
		ZHR: 60, PopulationIndex: 2.4, VelocityKmS: 66,
	},
	"southern_delta_aquariids": {
		Name: "Southern Delta Aquariids", Code: "SDA", ParentBody: "96P/Machholz (disputed)",
		RadiantRA: angle.Deg(339), RadiantDec: angle.Deg(-16),
		DriftRAPerDay: angle.Deg(1.0), DriftDecPerDay: angle.Deg(0.4),
		PeakSolarLongitude: 125, ActiveStartSolarLon: 125 - 16*meanSolarLongitudeDegPerDay, ActiveEndSolarLon: 125 + 22*meanSolarLongitudeDegPerDay,
		ZHR: 20, PopulationIndex: 3.2, VelocityKmS: 41,
	},
	"perseids": {
		Name: "Perseids", Code: "PER", ParentBody: "109P/Swift-Tuttle",
		RadiantRA: angle.Deg(46), RadiantDec: angle.Deg(58),
		DriftRAPerDay: angle.Deg(1.3), DriftDecPerDay: angle.Deg(0.15),
		PeakSolarLongitude: 140, ActiveStartSolarLon: 140 - 26*meanSolarLongitudeDegPerDay, ActiveEndSolarLon: 140 + 12*meanSolarLongitudeDegPerDay,
		ZHR: 100, PopulationIndex: 2.6, VelocityKmS: 59,
	},
	"orionids": {
		Name: "Orionids", Code: "ORI", ParentBody: "1P/Halley",
		RadiantRA: angle.Deg(95), RadiantDec: angle.Deg(16),
		DriftRAPerDay: angle.Deg(0.65), DriftDecPerDay: angle.Deg(0.05),
		PeakSolarLongitude: 208, ActiveStartSolarLon: 208 - 19*meanSolarLongitudeDegPerDay, ActiveEndSolarLon: 208 + 17*meanSolarLongitudeDegPerDay,
		ZHR: 23, PopulationIndex: 2.5, VelocityKmS: 66,
	},
	"leonids": {
		Name: "Leonids", Code: "LEO", ParentBody: "55P/Tempel-Tuttle",
		RadiantRA: angle.Deg(153), RadiantDec: angle.Deg(22),
		DriftRAPerDay: angle.Deg(0.6), DriftDecPerDay: angle.Deg(-0.4),
		PeakSolarLongitude: 235, ActiveStartSolarLon: 235 - 5*meanSolarLongitudeDegPerDay, ActiveEndSolarLon: 235 + 2*meanSolarLongitudeDegPerDay,
		ZHR: 15, PopulationIndex: 2.5, VelocityKmS: 71,
	},
	"geminids": {
		Name: "Geminids", Code: "GEM", ParentBody: "3200 Phaethon",
		RadiantRA: angle.Deg(112), RadiantDec: angle.Deg(33),
		DriftRAPerDay: angle.Deg(1.0), DriftDecPerDay: angle.Deg(-0.1),
		PeakSolarLongitude: 262, ActiveStartSolarLon: 262 - 7*meanSolarLongitudeDegPerDay, ActiveEndSolarLon: 262 + 3*meanSolarLongitudeDegPerDay,
		ZHR: 120, PopulationIndex: 2.6, VelocityKmS: 35,
	},
	"ursids": {
		Name: "Ursids", Code: "URS", ParentBody: "8P/Tuttle",
		RadiantRA: angle.Deg(217), RadiantDec: angle.Deg(76),
		DriftRAPerDay: angle.Zero(), DriftDecPerDay: angle.Zero(),
		PeakSolarLongitude: 270, ActiveStartSolarLon: 270 - 5*meanSolarLongitudeDegPerDay, ActiveEndSolarLon: 270 + 4*meanSolarLongitudeDegPerDay,
		ZHR: 10, PopulationIndex: 3.0, VelocityKmS: 33,
	},
}

// NewMeteorShower looks up name against MeteorShowers' map key (the
// normalized form of its Name) or any entry's Code, case- and
// space-insensitive, and returns it, or ErrUnknownMeteorShower if no entry
// matches.
func NewMeteorShower(name string) (MeteorShower, error) {
	want := normalizeSiteName(name)

	if m, ok := MeteorShowers[want]; ok {
		return m, nil
	}

	for _, m := range MeteorShowers {
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
func solarLongitudeInRange(lambda, start, end float64) bool {
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

// IsActive reports whether m is active at time t, based on the Sun's real
// computed ecliptic longitude of date (via the same solver Seasons uses),
// not a calendar-date range.
func (m MeteorShower) IsActive(t time.Time, prov eph.Provider) (bool, error) {
	lambda, err := sunEclipticLongitude(t, prov)
	if err != nil {
		return false, fmt.Errorf("meteor: active: %w", err)
	}

	return solarLongitudeInRange(lambda, m.ActiveStartSolarLon, m.ActiveEndSolarLon), nil
}

// RadiantAt returns m's radiant position at time t: the Sun's real
// ecliptic longitude of date is compared against m.PeakSolarLongitude,
// converted to an elapsed-day count via meanSolarLongitudeDegPerDay, and
// applied as linear RA/Dec drift from the peak position.
func (m MeteorShower) RadiantAt(t time.Time, prov eph.Provider) (ra, dec angle.Angle, err error) {
	lambda, err := sunEclipticLongitude(t, prov)
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

// ObservedRate returns the predicted number of m's meteors a single
// observer at site would see per hour at time t, given the sky conditions
// c describes (moonlight, zodiacal light, light pollution — the same
// LimitingMagnitudeConstraint used elsewhere in this package). This is
// IMO's own standard formula, inverted to predict rather than measure:
//
//	observedRate = ZHR · sin(h_R) · r^(LM − 6.5)
//
// where h_R is the radiant's altitude and LM is the site's actual
// limiting magnitude at that moment — under the defining standard
// conditions (h_R=90°, LM=6.5) this reduces to exactly ZHR. Returns 0
// (not an error) when the radiant is below the horizon.
func (m MeteorShower) ObservedRate(t time.Time, site *Site, prov eph.Provider, c LimitingMagnitudeConstraint) (float64, error) {
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

	radiant := NewStar(m.Name+" radiant", ra, dec)

	limMag, _, err := c.evaluate(radiant, t, ctx)
	if err != nil {
		return 0, fmt.Errorf("meteor: observed rate: %w", err)
	}

	if math.IsInf(limMag, -1) {
		return 0, nil
	}

	return m.ZHR * aa.Alt().Sin() * math.Pow(m.PopulationIndex, limMag-6.5), nil
}
