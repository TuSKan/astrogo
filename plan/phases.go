package plan

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// ── Moon Phases ──────────────────────────────────────────────────────────────

// MoonPhase identifies a primary lunar phase.
type MoonPhase int

const (
	PhaseNewMoon      MoonPhase = iota // Sun-Moon elongation = 0°
	PhaseFirstQuarter                  // Sun-Moon elongation = 90°
	PhaseFullMoon                      // Sun-Moon elongation = 180°
	PhaseLastQuarter                   // Sun-Moon elongation = 270°
)

func (p MoonPhase) String() string {
	switch p {
	case PhaseNewMoon:
		return "New Moon"
	case PhaseFirstQuarter:
		return "First Quarter"
	case PhaseFullMoon:
		return "Full Moon"
	case PhaseLastQuarter:
		return "Last Quarter"
	default:
		return "Unknown"
	}
}

// targetAngle returns the ecliptic elongation angle for this phase.
func (p MoonPhase) targetAngle() float64 {
	return float64(p) * 90.0
}

// MoonPhaseEvent records the precise instant of a primary lunar phase.
type MoonPhaseEvent struct {
	Time  time.Time
	Phase MoonPhase
}

// moonElongation returns the ecliptic longitude difference (Moon − Sun)
// normalized to [0, 360). This is the standard definition of lunar elongation
// used for phase computation.
func moonElongation(t time.Time, prov eph.Provider) (float64, error) {
	sunPos, err := eph.Position(prov, eph.Sun, t)
	if err != nil {
		return 0, fmt.Errorf("phases: sun position: %w", err)
	}
	moonPos, err := eph.Position(prov, eph.Moon, t)
	if err != nil {
		return 0, fmt.Errorf("phases: moon position: %w", err)
	}

	sunICRS, err := eph.ToICRS(sunPos)
	if err != nil {
		return 0, fmt.Errorf("phases: sun ICRS: %w", err)
	}
	moonICRS, err := eph.ToICRS(moonPos)
	if err != nil {
		return 0, fmt.Errorf("phases: moon ICRS: %w", err)
	}

	// Convert to ecliptic coordinates for elongation (TDB for SOFA)
	tdb := t.TDB()
	sunEcl := coord.ICRSToEcliptic(sunICRS, tdb)
	moonEcl := coord.ICRSToEcliptic(moonICRS, tdb)

	// Elongation = Moon longitude − Sun longitude, normalized to [0, 360)
	elong := moonEcl.Lon().Degrees() - sunEcl.Lon().Degrees()
	for elong < 0 {
		elong += 360
	}
	for elong >= 360 {
		elong -= 360
	}
	return elong, nil
}

// MoonPhases computes all primary lunar phases (New, First Quarter, Full,
// Last Quarter) in the time interval [start, end].
//
// The algorithm samples the Moon-Sun ecliptic elongation at regular intervals
// and uses Brent's method (via Solver) to refine the instant when the elongation
// crosses 0°, 90°, 180°, or 270°.
func MoonPhases(start, end time.Time, prov eph.Provider) ([]MoonPhaseEvent, error) {
	const step = 6 * time.Hour // ~4 samples per day → won't miss any phase
	solver := DefaultSolver()
	var events []MoonPhaseEvent

	phases := []MoonPhase{PhaseNewMoon, PhaseFirstQuarter, PhaseFullMoon, PhaseLastQuarter}

	prevElong, err := moonElongation(start, prov)
	if err != nil {
		return nil, err
	}

	prevT := start
	for t := start.Add(step); !t.After(end); t = t.Add(step) {
		curElong, err := moonElongation(t, prov)
		if err != nil {
			return nil, err
		}

		for _, phase := range phases {
			target := phase.targetAngle()

			if CrossesTarget(prevElong, curElong, target, 360) {
				eval := phaseEvaluator(target, prov)
				refined, _, err := solver.FindRoot(eval, prevT, t)
				if err != nil {
					continue
				}
				events = append(events, MoonPhaseEvent{Phase: phase, Time: refined})
			}
		}

		prevElong = curElong
		prevT = t
	}

	return events, nil
}

// phaseEvaluator returns an Evaluator that computes (elongation − target),
// normalized to [-180, 180], suitable for root-finding.
func phaseEvaluator(target float64, prov eph.Provider) Evaluator {
	return func(t time.Time) (float64, error) {
		elong, err := moonElongation(t, prov)
		if err != nil {
			return 0, err
		}
		diff := elong - target
		for diff > 180 {
			diff -= 360
		}
		for diff < -180 {
			diff += 360
		}
		return diff, nil
	}
}

// ── Earth's Seasons ──────────────────────────────────────────────────────────

// Season identifies a seasonal event.
type Season int

const (
	SeasonVernalEquinox   Season = iota // Sun ecliptic longitude = 0°
	SeasonSummerSolstice                // Sun ecliptic longitude = 90°
	SeasonAutumnalEquinox               // Sun ecliptic longitude = 180°
	SeasonWinterSolstice                // Sun ecliptic longitude = 270°
)

func (s Season) String() string {
	switch s {
	case SeasonVernalEquinox:
		return "Vernal Equinox"
	case SeasonSummerSolstice:
		return "Summer Solstice"
	case SeasonAutumnalEquinox:
		return "Autumnal Equinox"
	case SeasonWinterSolstice:
		return "Winter Solstice"
	default:
		return "Unknown"
	}
}

// targetLongitude returns the ecliptic longitude for this season.
func (s Season) targetLongitude() float64 {
	return float64(s) * 90.0
}

// SeasonEvent records the precise instant of a seasonal event.
type SeasonEvent struct {
	Time   time.Time
	Season Season
}

// sunEclipticLongitude returns the Sun's apparent ecliptic longitude at time t.
//
// SOFA's Eqec06 applies full IAU 2006 precession and IAU 2000A nutation,
// returning ecliptic coordinates of the TRUE equinox of date. For the
// Sun's apparent position, we subtract the aberration constant κ ≈ 20.496"
// (annual aberration displaces the Sun westward). Light-time and aberration
// largely cancel for the Sun, but the net effect shifts the apparent longitude
// by −κ in ecliptic coordinates.
func sunEclipticLongitude(t time.Time, prov eph.Provider) (float64, error) {
	sunPos, err := eph.Position(prov, eph.Sun, t)
	if err != nil {
		return 0, fmt.Errorf("seasons: sun position: %w", err)
	}
	sunICRS, err := eph.ToICRS(sunPos)
	if err != nil {
		return 0, fmt.Errorf("seasons: sun ICRS: %w", err)
	}

	tdb := t.TDB()

	// Eqec06: ICRS → ecliptic of TRUE equinox of date (precession + nutation)
	ecl := coord.ICRSToEcliptic(sunICRS, tdb)
	lon := ecl.Lon().Degrees()

	// Subtract aberration constant: apparent Sun longitude is ~20.5" west
	// of geometric due to Earth's orbital motion.
	const aberration = 20.496 / 3600.0 // degrees
	lon -= aberration

	// Normalize to [0, 360)
	for lon < 0 {
		lon += 360
	}
	for lon >= 360 {
		lon -= 360
	}
	return lon, nil
}

// Seasons computes all equinoxes and solstices for a given year.
// Returns events in chronological order.
func Seasons(year int, prov eph.Provider) ([]SeasonEvent, error) {
	start := time.Date(year, time.January, 1, 0, 0, 0, 0, time.LocationUTC)
	end := time.Date(year+1, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	const step = 24 * time.Hour // Daily sampling for ~1°/day Sun
	solver := DefaultSolver()
	var events []SeasonEvent

	seasons := []Season{SeasonVernalEquinox, SeasonSummerSolstice, SeasonAutumnalEquinox, SeasonWinterSolstice}

	prevLon, err := sunEclipticLongitude(start, prov)
	if err != nil {
		return nil, err
	}

	prevT := start
	for t := start.Add(step); !t.After(end); t = t.Add(step) {
		curLon, err := sunEclipticLongitude(t, prov)
		if err != nil {
			return nil, err
		}

		for _, season := range seasons {
			target := season.targetLongitude()

			if CrossesIncreasing(prevLon, curLon, target, 360) {
				eval := seasonEvaluator(target, prov)
				refined, _, err := solver.FindRoot(eval, prevT, t)
				if err != nil {
					continue
				}
				events = append(events, SeasonEvent{Season: season, Time: refined})
			}
		}

		prevLon = curLon
		prevT = t
	}

	return events, nil
}

// seasonEvaluator returns an Evaluator that computes (ecliptic longitude − target),
// normalized to [-180, 180], suitable for root-finding.
func seasonEvaluator(target float64, prov eph.Provider) Evaluator {
	return func(t time.Time) (float64, error) {
		lon, err := sunEclipticLongitude(t, prov)
		if err != nil {
			return 0, err
		}
		diff := lon - target
		for diff > 180 {
			diff -= 360
		}
		for diff < -180 {
			diff += 360
		}
		return diff, nil
	}
}

// ── Moon Illumination ────────────────────────────────────────────────────────

// MoonIllumination returns the fraction of the Moon's disk illuminated [0, 1]
// and the phase angle in degrees at time t.
func MoonIllumination(t time.Time, prov eph.Provider) (fraction float64, phaseAngle angle.Angle, err error) {
	sunPos, err := eph.Position(prov, eph.Sun, t)
	if err != nil {
		return 0, 0, err
	}
	moonPos, err := eph.Position(prov, eph.Moon, t)
	if err != nil {
		return 0, 0, err
	}

	sunICRS, err := eph.ToICRS(sunPos)
	if err != nil {
		return 0, 0, err
	}
	moonICRS, err := eph.ToICRS(moonPos)
	if err != nil {
		return 0, 0, err
	}

	// Phase angle = angular separation between Sun and Moon as seen from Earth
	sep := coord.Separation(moonICRS, sunICRS)

	// Illumination fraction = (1 - cos(phase_angle)) / 2
	frac := (1.0 - math.Cos(sep.Radians())) / 2.0

	return frac, sep, nil
}

// ── Earth's Apsides ─────────────────────────────────────────────────────────

// Apsis identifies an orbital apsis event.
type Apsis int

const (
	ApsisPerihelion Apsis = iota // Closest approach to the Sun
	ApsisAphelion                // Farthest point from the Sun
)

func (a Apsis) String() string {
	switch a {
	case ApsisPerihelion:
		return "Perihelion"
	case ApsisAphelion:
		return "Aphelion"
	default:
		return "Unknown"
	}
}

// ApsisEvent records the precise instant and distance of an orbital apsis.
type ApsisEvent struct {
	Time     time.Time
	Apsis    Apsis
	Distance float64
}

// Apsides computes the perihelion and aphelion of the Earth for a given year.
//
// Uses Brent's minimization (via Solver.FindExtremum) on the geocentric
// Earth-Sun distance. Perihelion occurs around January 3, aphelion around July 4.
func Apsides(year int, prov eph.Provider) ([]ApsisEvent, error) {
	solver := DefaultSolver()
	var events []ApsisEvent

	// Earth-Sun distance evaluator (returns distance in AU)
	sunDistance := func(t time.Time) (float64, error) {
		pos, err := eph.Position(prov, eph.Sun, t)
		if err != nil {
			return 0, fmt.Errorf("apsides: sun position: %w", err)
		}
		return pos.Norm(), nil
	}

	// Perihelion: minimum distance, typically early January
	// Search window: Dec 15 of previous year → Feb 15
	periStart := time.Date(year-1, time.December, 15, 0, 0, 0, 0, time.LocationUTC)
	periEnd := time.Date(year, time.February, 15, 0, 0, 0, 0, time.LocationUTC)
	periTime, periDist, err := solver.FindExtremum(Evaluator(sunDistance), periStart, periEnd, false)
	if err != nil {
		return nil, fmt.Errorf("apsides: perihelion: %w", err)
	}
	events = append(events, ApsisEvent{Apsis: ApsisPerihelion, Time: periTime, Distance: periDist})

	// Aphelion: maximum distance, typically early July
	// Search window: May 15 → Aug 15
	apStart := time.Date(year, time.May, 15, 0, 0, 0, 0, time.LocationUTC)
	apEnd := time.Date(year, time.August, 15, 0, 0, 0, 0, time.LocationUTC)
	apTime, apDist, err := solver.FindExtremum(Evaluator(sunDistance), apStart, apEnd, true)
	if err != nil {
		return nil, fmt.Errorf("apsides: aphelion: %w", err)
	}
	events = append(events, ApsisEvent{Apsis: ApsisAphelion, Time: apTime, Distance: apDist})

	return events, nil
}

// ── Eclipse Detection ────────────────────────────────────────────────────────

// EclipseType classifies an eclipse event.
type EclipseType int

const (
	EclipseLunar EclipseType = iota
	EclipseSolar
)

func (e EclipseType) String() string {
	switch e {
	case EclipseLunar:
		return "Lunar Eclipse"
	case EclipseSolar:
		return "Solar Eclipse"
	default:
		return "Unknown"
	}
}

// EclipseEvent records an eclipse with its time and ecliptic latitude.
type EclipseEvent struct {
	Time             time.Time
	Type             EclipseType
	EclipticLatitude angle.Angle
	Gamma            float64
}

// moonEclipticLatitude returns the Moon's ecliptic latitude at time t.
func moonEclipticLatitude(t time.Time, prov eph.Provider) (angle.Angle, error) {
	moonPos, err := eph.Position(prov, eph.Moon, t)
	if err != nil {
		return 0, fmt.Errorf("eclipse: moon position: %w", err)
	}
	moonICRS, err := eph.ToICRS(moonPos)
	if err != nil {
		return 0, fmt.Errorf("eclipse: moon ICRS: %w", err)
	}
	ecl := coord.ICRSToEcliptic(moonICRS, t.TDB())
	return ecl.Lat(), nil
}

// moonAntiSunSeparation returns the angular separation (degrees) between
// the Moon and the anti-solar point (Earth's shadow center).
// The "time of greatest lunar eclipse" is the minimum of this function.
func moonAntiSunSeparation(t time.Time, prov eph.Provider) (float64, error) {
	sunPos, err := eph.Position(prov, eph.Sun, t)
	if err != nil {
		return 0, err
	}
	moonPos, err := eph.Position(prov, eph.Moon, t)
	if err != nil {
		return 0, err
	}
	sunICRS, err := eph.ToICRS(sunPos)
	if err != nil {
		return 0, err
	}
	moonICRS, err := eph.ToICRS(moonPos)
	if err != nil {
		return 0, err
	}

	// Anti-solar point: flip the Sun's unit vector
	antiSun := coord.NewICRS(
		sunICRS.RA()+angle.Deg(180),
		-sunICRS.Dec(),
	)
	sep := coord.Separation(moonICRS, antiSun)
	return sep.Degrees(), nil
}

// moonSunSeparation returns the angular separation (degrees) between
// the Moon and the Sun as seen from Earth.
// The "time of greatest solar eclipse" is the minimum of this function.
func moonSunSeparation(t time.Time, prov eph.Provider) (float64, error) {
	sunPos, err := eph.Position(prov, eph.Sun, t)
	if err != nil {
		return 0, err
	}
	moonPos, err := eph.Position(prov, eph.Moon, t)
	if err != nil {
		return 0, err
	}
	sunICRS, err := eph.ToICRS(sunPos)
	if err != nil {
		return 0, err
	}
	moonICRS, err := eph.ToICRS(moonPos)
	if err != nil {
		return 0, err
	}
	return coord.Separation(moonICRS, sunICRS).Degrees(), nil
}

// LunarEclipses finds potential lunar eclipses in [start, end] by identifying
// Full Moons where the Moon's ecliptic latitude is within the Danjon limit
// (≈1.58° for penumbral, ≈1.05° for partial, ≈0.55° for total).
//
// The eclipse time is refined to the moment of minimum Moon–anti-Sun angular
// separation (time of greatest eclipse). The Gamma field indicates how central
// the eclipse is (0 = perfectly central, 1 = at the limit).
func LunarEclipses(start, end time.Time, prov eph.Provider) ([]EclipseEvent, error) {
	const penumbralLimit = 1.58 // degrees

	phases, err := MoonPhases(start, end, prov)
	if err != nil {
		return nil, fmt.Errorf("lunar eclipses: %w", err)
	}

	solver := DefaultSolver()
	var eclipses []EclipseEvent
	for _, phase := range phases {
		if phase.Phase != PhaseFullMoon {
			continue
		}

		lat, err := moonEclipticLatitude(phase.Time, prov)
		if err != nil {
			continue
		}

		absLat := math.Abs(lat.Degrees())
		if absLat <= penumbralLimit {
			// Refine: minimize Moon–anti-Sun angular separation in a ±30 min
			// window around the syzygy. This finds the "time of greatest eclipse".
			tMin := phase.Time.Add(-30 * time.Minute)
			tMax := phase.Time.Add(30 * time.Minute)
			eclTime, _, err := solver.FindExtremum(func(t time.Time) (float64, error) {
				return moonAntiSunSeparation(t, prov)
			}, tMin, tMax, false)
			if err != nil {
				eclTime = phase.Time
			}

			refinedLat, _ := moonEclipticLatitude(eclTime, prov)
			eclipses = append(eclipses, EclipseEvent{
				Type:             EclipseLunar,
				Time:             eclTime,
				EclipticLatitude: refinedLat,
				Gamma:            math.Abs(refinedLat.Degrees()) / penumbralLimit,
			})
		}
	}

	return eclipses, nil
}

// SolarEclipses finds potential solar eclipses in [start, end] by identifying
// New Moons where the Moon's ecliptic latitude is within the solar eclipse limit
// (≈1.58° for partial, ≈0.99° for total/annular).
//
// The eclipse time is refined to the moment of minimum Moon–Sun angular
// separation (time of greatest eclipse). The Gamma field indicates how central
// the eclipse is (0 = perfectly central, 1 = at the limit).
func SolarEclipses(start, end time.Time, prov eph.Provider) ([]EclipseEvent, error) {
	const partialLimit = 1.58 // degrees

	phases, err := MoonPhases(start, end, prov)
	if err != nil {
		return nil, fmt.Errorf("solar eclipses: %w", err)
	}

	solver := DefaultSolver()
	var eclipses []EclipseEvent
	for _, phase := range phases {
		if phase.Phase != PhaseNewMoon {
			continue
		}

		lat, err := moonEclipticLatitude(phase.Time, prov)
		if err != nil {
			continue
		}

		absLat := math.Abs(lat.Degrees())
		if absLat <= partialLimit {
			// Refine: minimize Moon–Sun angular separation in a ±30 min
			// window around the syzygy. This finds the "time of greatest eclipse".
			tMin := phase.Time.Add(-30 * time.Minute)
			tMax := phase.Time.Add(30 * time.Minute)
			eclTime, _, err := solver.FindExtremum(func(t time.Time) (float64, error) {
				return moonSunSeparation(t, prov)
			}, tMin, tMax, false)
			if err != nil {
				eclTime = phase.Time
			}

			refinedLat, _ := moonEclipticLatitude(eclTime, prov)
			eclipses = append(eclipses, EclipseEvent{
				Type:             EclipseSolar,
				Time:             eclTime,
				EclipticLatitude: refinedLat,
				Gamma:            math.Abs(refinedLat.Degrees()) / partialLimit,
			})
		}
	}

	return eclipses, nil
}
