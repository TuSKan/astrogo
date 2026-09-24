package plan

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// ── Moon Phases ──────────────────────────────────────────────────────────────

// MoonPhase identifies a primary lunar phase.
type MoonPhase int

const (
	// PhaseNewMoon is Sun-Moon elongation = 0°.
	PhaseNewMoon MoonPhase = iota
	// PhaseFirstQuarter is Sun-Moon elongation = 90°.
	PhaseFirstQuarter
	// PhaseFullMoon is Sun-Moon elongation = 180°.
	PhaseFullMoon
	// PhaseLastQuarter is Sun-Moon elongation = 270°.
	PhaseLastQuarter
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
	if prov == nil {
		prov = eph.Default()
	}

	step := unit.Hours(6) // ~4 samples per day → won't miss any phase

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
	// SeasonVernalEquinox is when Sun ecliptic longitude = 0°.
	SeasonVernalEquinox Season = iota
	// SeasonSummerSolstice is when Sun ecliptic longitude = 90°.
	SeasonSummerSolstice
	// SeasonAutumnalEquinox is when Sun ecliptic longitude = 180°.
	SeasonAutumnalEquinox
	// SeasonWinterSolstice is when Sun ecliptic longitude = 270°.
	SeasonWinterSolstice
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
	if prov == nil {
		prov = eph.Default()
	}

	start := time.Date(year, time.January, 1, 0, 0, 0, 0, time.LocationUTC)
	end := time.Date(year+1, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	step := unit.Hours(24) // Daily sampling for ~1°/day Sun

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
	if prov == nil {
		prov = eph.Default()
	}

	sunPos, err := eph.Position(prov, eph.Sun, t)
	if err != nil {
		return 0, 0, fmt.Errorf("illumination: sun position: %w", err)
	}

	moonPos, err := eph.Position(prov, eph.Moon, t)
	if err != nil {
		return 0, 0, fmt.Errorf("illumination: moon position: %w", err)
	}

	sunICRS, err := eph.ToICRS(sunPos)
	if err != nil {
		return 0, 0, fmt.Errorf("illumination: sun ICRS: %w", err)
	}

	moonICRS, err := eph.ToICRS(moonPos)
	if err != nil {
		return 0, 0, fmt.Errorf("illumination: moon ICRS: %w", err)
	}

	// Phase angle = angular separation between Sun and Moon as seen from Earth
	sep := coord.Separation(moonICRS, sunICRS)

	// Illumination fraction = (1 - cos(phase_angle)) / 2
	frac := (1.0 - math.Cos(sep.Radians())) / 2.0

	return frac, sep, nil
}

// MoonElongation returns the Moon's ecliptic elongation from the Sun at
// time t: the Moon's ecliptic longitude minus the Sun's, normalized to
// [0°, 360°). 0° at new moon, 90° at first quarter, 180° at full moon, 270°
// at last quarter — monotonically increasing across a full lunation, unlike
// [MoonIllumination]'s phaseAngle (the Sun–Moon–observer separation, which
// is symmetric about full and so takes the same value on both the waxing
// and waning side of a lunation). Use this — or [MoonPhaseFraction] — for
// "is tonight's Moon waxing or waning", which phaseAngle alone can't answer.
func MoonElongation(t time.Time, prov eph.Provider) (angle.Angle, error) {
	if prov == nil {
		prov = eph.Default()
	}

	elongDeg, err := moonElongation(t, prov)
	if err != nil {
		return 0, err
	}

	return angle.Deg(elongDeg), nil
}

// MoonPhaseFraction returns the Moon's position in its current synodic
// cycle as a continuous fraction: 0.0 at new moon, 0.25 at first quarter,
// 0.5 at full moon, 0.75 at last quarter, approaching 1.0 as the next new
// moon nears. Waxing corresponds to [0, 0.5), waning to [0.5, 1) — this one
// number carries both cycle position and waxing/waning, which is usually
// what a presentation layer actually wants; MoonElongation is the same
// information in degrees, for callers who want it in that form instead.
func MoonPhaseFraction(t time.Time, prov eph.Provider) (float64, error) {
	elong, err := MoonElongation(t, prov)
	if err != nil {
		return 0, err
	}

	return elong.Degrees() / 360.0, nil
}

// ── Earth's Apsides ─────────────────────────────────────────────────────────

// Apsis identifies an orbital apsis event.
type Apsis int

const (
	// ApsisPerihelion is the closest approach to the Sun.
	ApsisPerihelion Apsis = iota
	// ApsisAphelion is the farthest point from the Sun.
	ApsisAphelion
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
	Distance unit.Length
}

// Apsides computes the perihelion and aphelion of the Earth for a given year.
//
// Uses Brent's minimization (via Solver.FindExtremum) on the geocentric
// Earth-Sun distance. Perihelion occurs around January 3, aphelion around July 4.
func Apsides(year int, prov eph.Provider) ([]ApsisEvent, error) {
	if prov == nil {
		prov = eph.Default()
	}

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

	events = append(events, ApsisEvent{Apsis: ApsisPerihelion, Time: periTime, Distance: unit.AU(periDist)})

	// Aphelion: maximum distance, typically early July
	// Search window: May 15 → Aug 15
	apStart := time.Date(year, time.May, 15, 0, 0, 0, 0, time.LocationUTC)
	apEnd := time.Date(year, time.August, 15, 0, 0, 0, 0, time.LocationUTC)

	apTime, apDist, err := solver.FindExtremum(Evaluator(sunDistance), apStart, apEnd, true)
	if err != nil {
		return nil, fmt.Errorf("apsides: aphelion: %w", err)
	}

	events = append(events, ApsisEvent{Apsis: ApsisAphelion, Time: apTime, Distance: unit.AU(apDist)})

	return events, nil
}

// ── Eclipse Detection ────────────────────────────────────────────────────────

// EclipseType classifies an eclipse event.
type EclipseType int

const (
	// EclipseLunar is when the Moon passes through Earth's shadow.
	EclipseLunar EclipseType = iota
	// EclipseSolar is when the Moon passes between Earth and Sun.
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

// EclipseKind is what kind of eclipse an EclipseEvent is, as NASA's Five
// Millennium Canons classify them: penumbral, partial or total for the Moon;
// partial, annular, total or hybrid for the Sun. The zero value is no kind.
type EclipseKind int

const (
	// EclipsePenumbral is a lunar eclipse in which the Moon enters only
	// Earth's penumbra.
	EclipsePenumbral EclipseKind = iota + 1
	// EclipsePartial is a lunar eclipse in which part of the Moon enters the
	// umbra, or a solar eclipse that is total or annular nowhere on Earth.
	EclipsePartial
	// EclipseTotal is a lunar eclipse with the whole Moon in the umbra, or a
	// solar eclipse whose umbra reaches the Earth.
	EclipseTotal
	// EclipseAnnular is a solar eclipse whose antumbra reaches the Earth: the
	// Moon, too small to cover the Sun, leaves a ring.
	EclipseAnnular
	// EclipseHybrid is a solar eclipse that is total along part of its path
	// and annular along the rest.
	EclipseHybrid
)

func (k EclipseKind) String() string {
	switch k {
	case EclipsePenumbral:
		return "Penumbral"
	case EclipsePartial:
		return "Partial"
	case EclipseTotal:
		return "Total"
	case EclipseAnnular:
		return "Annular"
	case EclipseHybrid:
		return "Hybrid"
	default:
		return "Unknown"
	}
}

// EclipseEvent records an eclipse: when it is greatest, what kind it is, and
// how deep.
type EclipseEvent struct {
	Time             time.Time
	Type             EclipseType
	EclipticLatitude angle.Angle
	Gamma            float64

	// Kind is the eclipse's kind, decided as the canon decides it: for the
	// Moon by the umbral magnitude, for the Sun by whether the umbra or the
	// antumbra reaches the Earth, and where. Until #405 there was none, and
	// callers guessed it from EclipticLatitude, which cannot know it.
	Kind EclipseKind

	// Magnitude is the canon's eclipse magnitude at greatest eclipse.
	//
	// For the Moon it is the umbral magnitude: the fraction of the Moon's
	// diameter inside Earth's umbra, negative for a penumbral eclipse by the
	// fraction the Moon stays outside it.
	//
	// For the Sun it is taken at the point of greatest eclipse. For a central
	// eclipse — total, annular or hybrid, the shadow axis meeting the Earth —
	// it is the ratio of the Moon's apparent diameter to the Sun's there: over
	// 1 for total, under 1 for annular. Otherwise it is the fraction of the
	// Sun's diameter covered at the point of the limb nearest the axis, which
	// is also what the canon gives for the rare total or annular eclipse
	// whose axis misses the Earth.
	Magnitude float64

	// PenumbralMagnitude is, for the Moon, the fraction of its diameter inside
	// Earth's penumbra; it is above 1 in every total eclipse. Zero for the Sun.
	PenumbralMagnitude float64
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

// LunarEclipses returns the lunar eclipses in [start, end]: every Full Moon at
// which the Moon enters Earth's penumbra, penumbral eclipses included.
//
// Earth's shadow is sized as NASA's Five Millennium Canon of Lunar Eclipses
// sizes it, by Danjon's rule, from that month's distances to the Moon and the
// Sun, so an eclipse here is an eclipse there; against six centuries of the
// canon the two agree on every one. Time is greatest eclipse, when the Moon's
// center passes closest to the shadow axis. Gamma is that closest distance as
// a fraction of the distance at which the Moon would just graze the penumbra:
// 0 central, 1 grazing. EclipticLatitude is the Moon's, at greatest eclipse.
//
// Until #401 an eclipse was any Full Moon within a fixed 1.58° of ecliptic
// latitude, which is not a property of the shadow: it reported eclipses that
// do not happen and missed some that do.
func LunarEclipses(start, end time.Time, prov eph.Provider) ([]EclipseEvent, error) {
	if prov == nil {
		prov = eph.Default()
	}

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
			return nil, fmt.Errorf("lunar eclipses: %w", err)
		}

		if math.Abs(lat.Degrees()) > eclipseLatitudeScreen {
			continue
		}

		eclTime, _, err := solver.FindExtremum(func(t time.Time) (float64, error) {
			g, err := newEclipseGeometry(t, prov)
			if err != nil {
				return 0, err
			}

			return g.lunarAxisDistance(), nil
		}, phase.Time.Add(unit.Minutes(-eclipseSearchHalfWidth)), phase.Time.Add(unit.Minutes(eclipseSearchHalfWidth)), false)
		if err != nil {
			return nil, fmt.Errorf("lunar eclipses: greatest eclipse near %v: %w", phase.Time, err)
		}

		g, err := newEclipseGeometry(eclTime, prov)
		if err != nil {
			return nil, fmt.Errorf("lunar eclipses: %w", err)
		}

		penumbral, umbral, gamma := g.lunarMagnitudes()
		if penumbral <= 0 {
			continue
		}

		eclLat, err := moonEclipticLatitude(eclTime, prov)
		if err != nil {
			return nil, fmt.Errorf("lunar eclipses: %w", err)
		}

		eclipses = append(eclipses, EclipseEvent{
			Type:               EclipseLunar,
			Time:               eclTime,
			EclipticLatitude:   eclLat,
			Gamma:              gamma,
			Kind:               lunarKind(umbral),
			Magnitude:          umbral,
			PenumbralMagnitude: penumbral,
		})
	}

	return eclipses, nil
}

// SolarEclipses returns the solar eclipses in [start, end]: every New Moon at
// which the Moon's penumbra falls on the Earth, partial eclipses included.
//
// The penumbra is sized, and the Earth shaped, as NASA's Five Millennium Canon
// of Solar Eclipses has them, so an eclipse here is an eclipse there; against
// six centuries of the canon the two agree on every one. Time is greatest
// eclipse, when the shadow's axis passes closest to Earth's center. Gamma is
// that closest distance as a fraction of the distance at which the penumbra
// would just graze Earth's limb: 0 central, 1 grazing. EclipticLatitude is the
// Moon's, at greatest eclipse.
//
// Until #401 an eclipse was any New Moon within a fixed 1.58° of ecliptic
// latitude, which reported eclipses that do not happen.
func SolarEclipses(start, end time.Time, prov eph.Provider) ([]EclipseEvent, error) {
	if prov == nil {
		prov = eph.Default()
	}

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
			return nil, fmt.Errorf("solar eclipses: %w", err)
		}

		if math.Abs(lat.Degrees()) > eclipseLatitudeScreen {
			continue
		}

		eclTime, _, err := solver.FindExtremum(func(t time.Time) (float64, error) {
			g, err := newEclipseGeometry(t, prov)
			if err != nil {
				return 0, err
			}

			return g.solarAxisDistance(), nil
		}, phase.Time.Add(unit.Minutes(-eclipseSearchHalfWidth)), phase.Time.Add(unit.Minutes(eclipseSearchHalfWidth)), false)
		if err != nil {
			return nil, fmt.Errorf("solar eclipses: greatest eclipse near %v: %w", phase.Time, err)
		}

		g, err := newEclipseGeometry(eclTime, prov)
		if err != nil {
			return nil, fmt.Errorf("solar eclipses: %w", err)
		}

		shadow := g.solarShadow(eclTime)

		margin, gamma := shadow.margin()
		if margin <= 0 {
			continue
		}

		kind, magnitude, err := solarEclipseKind(prov, eclTime, shadow)
		if err != nil {
			return nil, fmt.Errorf("solar eclipses: %w", err)
		}

		eclLat, err := moonEclipticLatitude(eclTime, prov)
		if err != nil {
			return nil, fmt.Errorf("solar eclipses: %w", err)
		}

		eclipses = append(eclipses, EclipseEvent{
			Type:             EclipseSolar,
			Time:             eclTime,
			EclipticLatitude: eclLat,
			Gamma:            gamma,
			Kind:             kind,
			Magnitude:        magnitude,
		})
	}

	return eclipses, nil
}
