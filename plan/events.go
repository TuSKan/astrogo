package plan

import (
	"fmt"
	"math"
	"sort"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris"

	"github.com/TuSKan/astrogo/time"
)

// EventFamily classifies the broad category of an astronomical event.
type EventFamily int

const (
	// EventFamilyVisibility encompasses events related to an observer's local horizon (e.g. rise, set, transit).
	EventFamilyVisibility EventFamily = iota
	// EventFamilyRelativeGeometry encompasses events dependent on the angular relationship
	// between bodies (e.g. conjunction, opposition, greatest elongation).
	EventFamilyRelativeGeometry
	// EventFamilyOverlap encompasses events where physical bodies obscure one another (e.g. eclipses, occultations).
	EventFamilyOverlap
	// EventFamilyIllumination encompasses events related to phase and illumination (e.g. moon phases).
	EventFamilyIllumination
)

func (f EventFamily) String() string {
	switch f {
	case EventFamilyVisibility:
		return "Visibility"
	case EventFamilyRelativeGeometry:
		return "Relative Geometry"
	case EventFamilyOverlap:
		return "Overlap"
	case EventFamilyIllumination:
		return "Illumination"
	default:
		return "Unknown"
	}
}

// EventKind identifies the specific astronomical event within a family.
type EventKind int

const (
	// -- FamilyVisibility --

	// EventRise represents the target crossing upward through the threshold altitude.
	EventRise EventKind = iota
	// EventSet represents the target crossing downward through the threshold altitude.
	EventSet
	// EventTransit represents the target reaching its local maximum altitude.
	EventTransit

	// -- FamilyRelativeGeometry --

	// EventConjunction represents two targets having the same right ascension (ΔRA = 0).
	EventConjunction
	// EventConjunctionEcliptic represents two targets having the same ecliptic longitude (Δλ = 0).
	EventConjunctionEcliptic
	// EventAppulse represents the moment of minimum angular separation between two targets.
	EventAppulse
	// EventOpposition represents two targets having apparent longitudes or right ascensions 180 degrees apart.
	EventOpposition
	// EventGreatestElongationEast represents the maximum angular separation of an inner planet east of the Sun.
	EventGreatestElongationEast
	// EventGreatestElongationWest represents the maximum angular separation of an inner planet west of the Sun.
	EventGreatestElongationWest
	// EventQuadratureEast represents a target being 90 degrees east of the Sun.
	EventQuadratureEast
	// EventQuadratureWest represents a target being 90 degrees west of the Sun.
	EventQuadratureWest

	// -- FamilyOverlap (Phase 2/3) --

	EventOccultationStart
	EventOccultationEnd
	EventEclipseStart
	EventEclipseEnd
	EventIngress
	EventEgress

	// -- FamilyIllumination (Phase 3) --

	EventNewMoon
	EventFirstQuarter
	EventFullMoon
	EventLastQuarter
	EventMaxIllumination
	EventMinIllumination
)

const (
	// EventAnyVisibility is a wildcard to find Rise, Set, and Transit in a single pass.
	EventAnyVisibility EventKind = -1
)

func (k EventKind) String() string {
	switch k {
	case EventRise:
		return "Rise"
	case EventSet:
		return "Set"
	case EventTransit:
		return "Transit"
	case EventConjunction:
		return "Conjunction"
	case EventConjunctionEcliptic:
		return "Conjunction (Ecl. Lon.)"
	case EventAppulse:
		return "Appulse"
	case EventOpposition:
		return "Opposition"
	case EventGreatestElongationEast:
		return "Greatest Elongation East"
	case EventGreatestElongationWest:
		return "Greatest Elongation West"
	case EventQuadratureEast:
		return "Quadrature East"
	case EventQuadratureWest:
		return "Quadrature West"
	case EventOccultationStart:
		return "Occultation Start"
	case EventOccultationEnd:
		return "Occultation End"
	case EventEclipseStart:
		return "Eclipse Start"
	case EventEclipseEnd:
		return "Eclipse End"
	case EventIngress:
		return "Ingress"
	case EventEgress:
		return "Egress"
	case EventNewMoon:
		return "New Moon"
	case EventFirstQuarter:
		return "First Quarter"
	case EventFullMoon:
		return "Full Moon"
	case EventLastQuarter:
		return "Last Quarter"
	case EventMaxIllumination:
		return "Max Illumination"
	case EventMinIllumination:
		return "Min Illumination"
	default:
		return "Unknown"
	}
}

// EventSpec formally defines what type of astronomical event is being solved for.
type EventSpec struct {
	// Family defines the broad category of the event solver.
	Family EventFamily

	// Kind identifies the specific event type being solved.
	Kind EventKind

	// Target is the primary object for the event (e.g., the Sun, the Moon, a Star).
	Target Observable

	// Other is an optional secondary target used for relative events (e.g. Conjunctions, Eclipses).
	Other Observable

	// Observer is required for topocentric (site-dependent) events like Rise and Set.
	// Relative geometry events (Conjunction) might omit this to solve geocentrically.
	Observer *Site

	// Threshold defines the angular condition for the event.
	// For rise/set, this is the horizon altitude.
	// For geometry, it might represent a specific separation angle.
	Threshold angle.Angle
}

// Validate checks if the Spec configuration is fully provided for its type.
func (s EventSpec) Validate() error {
	if s.Target == nil {
		return fmt.Errorf("event spec must contain a primary target")
	}

	switch s.Family {
	case EventFamilyVisibility:
		if s.Observer == nil {
			return fmt.Errorf("visibility events require an observer geodetic location")
		}
	case EventFamilyRelativeGeometry, EventFamilyOverlap:
		if s.Other == nil && !isPhaseEvent(s.Kind) {
			return fmt.Errorf("%v geometry requires a secondary target", s.Kind)
		}
	case EventFamilyIllumination:
		// Illumination events need only a Target; the Sun is implicit.
	}
	return nil
}

// isPhaseEvent returns true for EventKinds in the Illumination family.
func isPhaseEvent(k EventKind) bool {
	switch k {
	case EventNewMoon, EventFirstQuarter, EventFullMoon, EventLastQuarter,
		EventMaxIllumination, EventMinIllumination:
		return true
	}
	return false
}

// evaluator is a function that returns the metric to be solved (e.g. altitude diff, separation angle).
type evaluator func(t time.Time) (float64, error)

// EventSolver searches for astronomical events based on an EventSpec over a time interval.
type EventSolver struct {
	Step   time.Duration
	Solver Solver
}

// NewEventSolver creates a numerical solver for finding events.
func NewEventSolver(step, tol time.Duration) EventSolver {
	if step <= 0 {
		step = 15 * time.Minute
	}
	if tol <= 0 {
		tol = 1 * time.Second
	}
	return EventSolver{
		Step: step,
		Solver: Solver{
			Tolerance: tol,
			MaxIter:   64,
		},
	}
}

// Find searches for events matching the given specification within the interval.
func (s EventSolver) Find(spec EventSpec, start, end time.Time) ([]Event, error) {
	if err := spec.Validate(); err != nil {
		return nil, err
	}

	var events []Event
	var err error

	switch spec.Family {
	case EventFamilyVisibility:
		events, err = s.solveVisibility(spec, start, end)
	case EventFamilyRelativeGeometry:
		events, err = s.solveGeometry(spec, start, end)
	case EventFamilyIllumination:
		events, err = s.solveIllumination(spec, start, end)
	default:
		return nil, fmt.Errorf("event solver for family %v is not implemented", spec.Family)
	}

	if err != nil {
		return nil, err
	}

	// Sort events by time
	sort.Slice(events, func(i, j int) bool {
		return events[i].Time.Before(events[j].Time)
	})

	return events, nil
}

// refineRoot delegates to the unified Solver.FindRoot.
func (s EventSolver) refineRoot(eval evaluator, t1, t2 time.Time, _ float64) (time.Time, float64, error) {
	return s.Solver.FindRoot(Evaluator(eval), t1, t2)
}

// refineExtremum delegates to the unified Solver.FindExtremum.
func (s EventSolver) refineExtremum(eval evaluator, t1, t3 time.Time, isMax bool) (time.Time, float64, error) {
	return s.Solver.FindExtremum(Evaluator(eval), t1, t3, isMax)
}

func (s EventSolver) solveVisibility(spec EventSpec, start, end time.Time) ([]Event, error) {
	var events []Event

	evalVal := func(t time.Time) (float64, error) {
		ctx := coord.NewContext(t, spec.Observer.Location(), spec.Observer.Atmosphere())

		// For solar system bodies, use the vector-based topocentric pipeline
		// which properly corrects for diurnal parallax (critical for the Moon: ~1°).
		if body, ok := spec.Target.(Body); ok {
			vec, err := body.GeocentricVec(t)
			if err != nil {
				return 0, err
			}
			aa := ctx.GeocentricToObserved(vec)
			return aa.Alt().Degrees() - spec.Threshold.Degrees(), nil
		}

		// For deep-space / stellar targets, use the astrometric pipeline.
		pos, err := spec.Target.Position(t)
		if err != nil {
			return 0, err
		}
		aa, err := ctx.ICRSToAltAz(pos)
		if err != nil {
			return 0, err
		}
		return aa.Alt().Degrees() - spec.Threshold.Degrees(), nil
	}

	n := int(end.Sub(start)/s.Step) + 2
	times := make([]time.Time, 0, n)
	alts := make([]float64, 0, n)

	for t := start; !t.After(end); t = t.Add(s.Step) {
		times = append(times, t)
		h, err := evalVal(t)
		if err != nil {
			return nil, err
		}
		alts = append(alts, h)
	}
	if last := times[len(times)-1]; last.Before(end) {
		times = append(times, end)
		h, err := evalVal(end)
		if err != nil {
			return nil, err
		}
		alts = append(alts, h)
	}

	for i := 0; i < len(times)-1; i++ {
		t1, t2 := times[i], times[i+1]
		h1, h2 := alts[i], alts[i+1]

		// Crossings (Rise/Set)
		if (h1 <= 0 && h2 > 0) || (h1 > 0 && h2 <= 0) {
			kind := EventRise
			if h1 > 0 {
				kind = EventSet
			}

			if spec.Kind == kind || spec.Kind == EventAnyVisibility {
				resTime, _, err := s.refineRoot(evalVal, t1, t2, h1)
				if err != nil {
					return nil, err
				}

				// Calculate geometric outputs using the appropriate pipeline
				resCtx := coord.NewContext(resTime, spec.Observer.Location(), spec.Observer.Atmosphere())
				var aa *coord.AltAz
				if body, ok := spec.Target.(Body); ok {
					vec, _ := body.GeocentricVec(resTime)
					aa = resCtx.GeocentricToObserved(vec)
				} else {
					pos, _ := spec.Target.Position(resTime)
					aa, _ = resCtx.ICRSToAltAz(pos)
				}

				events = append(events, Event{
					Kind:              kind,
					Time:              resTime,
					Altitude:          aa.Alt(),
					GeometricAltitude: aa.Alt(),
					Azimuth:           aa.Az(),
					Value:             aa.Alt().Degrees() - spec.Threshold.Degrees(),
				})
			}
		}

		// Local Maximum (Transit) — refine via hour angle = 0
		if i > 0 {
			h0 := alts[i-1]
			if h1 > h0 && h1 >= h2 && (spec.Kind == EventTransit || spec.Kind == EventAnyVisibility) {
				// Use hour angle root-finding instead of altitude maximization.
				// HA crosses zero sharply at transit, giving robust sub-second convergence
				// even for near-zenith transits where altitude is flat.
				evalHA := func(t time.Time) (float64, error) {
					pos, err := spec.Target.Position(t)
					if err != nil {
						return 0, err
					}
					ctx := coord.NewContext(t, spec.Observer.Location(), spec.Observer.Atmosphere())
					ha, err := ctx.ICRSToHourAngle(pos)
					if err != nil {
						return 0, err
					}
					return ha.Degrees(), nil
				}

				// Compute HA at bracket endpoints to find the sub-bracket containing HA=0
				haLeft, _ := evalHA(times[i-1])
				haMid, _ := evalHA(times[i])
				haRight, _ := evalHA(times[i+1])

				var bracketA, bracketB time.Time
				if haLeft*haMid <= 0 {
					bracketA, bracketB = times[i-1], times[i]
				} else if haMid*haRight <= 0 {
					bracketA, bracketB = times[i], times[i+1]
				} else {
					// HA doesn't cross zero — no transit in this bracket
					continue
				}

				resTime, _, err := s.refineRoot(evalHA, bracketA, bracketB, 0)
				if err != nil {
					continue // Skip if solver fails
				}

				resCtx := coord.NewContext(resTime, spec.Observer.Location(), spec.Observer.Atmosphere())
				pos, _ := spec.Target.Position(resTime)
				aa, _ := resCtx.ICRSToAltAz(pos)

				events = append(events, Event{
					Kind:              EventTransit,
					Time:              resTime,
					Altitude:          aa.Alt(),
					GeometricAltitude: aa.Alt(),
					Azimuth:           aa.Az(),
					Value:             aa.Alt().Degrees(),
				})
			}
		}
	}

	return events, nil
}

func (s EventSolver) solveGeometry(spec EventSpec, start, end time.Time) ([]Event, error) {
	var events []Event

	// Geometry solver handles Conjunction, Opposition, and Greatest Elongation.
	evalVal := func(t time.Time) (float64, error) {
		pos1, err := spec.Target.Position(t)
		if err != nil {
			return 0, err
		}
		pos2, err := spec.Other.Position(t)
		if err != nil {
			return 0, err
		}

		switch spec.Kind {
		case EventConjunction, EventOpposition:
			// Difference in Right Ascension
			diff := pos1.RA().Degrees() - pos2.RA().Degrees()
			// Normalize to [-180, 180]
			for diff > 180 {
				diff -= 360
			}
			for diff <= -180 {
				diff += 360
			}

			if spec.Kind == EventOpposition {
				if diff > 0 {
					diff -= 180
				} else {
					diff += 180
				}
			}
			return diff, nil
		case EventConjunctionEcliptic:
			// Difference in Ecliptic Longitude
			ecl1 := coord.ICRSToEcliptic(pos1, t)
			ecl2 := coord.ICRSToEcliptic(pos2, t)
			diff := ecl1.Lon().Degrees() - ecl2.Lon().Degrees()
			for diff > 180 {
				diff -= 360
			}
			for diff <= -180 {
				diff += 360
			}
			return diff, nil
		case EventAppulse:
			// Angular separation (for minimum-finding)
			sep := coord.Separation(pos1, pos2).Degrees()
			return sep, nil
		case EventGreatestElongationEast, EventGreatestElongationWest:
			sep := coord.Separation(pos1, pos2).Degrees()
			return sep, nil
		default:
			return 0, fmt.Errorf("unsupported geometry kind: %v", spec.Kind)
		}
	}

	n := int(end.Sub(start)/s.Step) + 2
	times := make([]time.Time, 0, n)
	vals := make([]float64, 0, n)

	for t := start; !t.After(end); t = t.Add(s.Step) {
		times = append(times, t)
		v, err := evalVal(t)
		if err != nil {
			return nil, err
		}
		vals = append(vals, v)
	}
	if last := times[len(times)-1]; last.Before(end) {
		times = append(times, end)
		v, err := evalVal(end)
		if err != nil {
			return nil, err
		}
		vals = append(vals, v)
	}

	for i := 0; i < len(times)-1; i++ {
		t1, t2 := times[i], times[i+1]
		v1, v2 := vals[i], vals[i+1]

		if spec.Kind == EventConjunction || spec.Kind == EventConjunctionEcliptic || spec.Kind == EventOpposition {
			// Find root (crosses 0)
			if (v1 <= 0 && v2 > 0) || (v1 > 0 && v2 <= 0) {
				// Handle 180/-180 wrap-around false crossings if they jump significantly > 180
				if math.Abs(v1-v2) < 180 {
					resTime, val, err := s.refineRoot(evalVal, t1, t2, v1)
					if err != nil {
						return nil, err
					}
					events = append(events, Event{
						Kind:  spec.Kind,
						Time:  resTime,
						Value: val,
					})
				}
			}
		}

		if (spec.Kind == EventGreatestElongationEast || spec.Kind == EventGreatestElongationWest) && i > 0 {
			// Find local maximum
			v0 := vals[i-1]
			if v1 > v0 && v1 >= v2 {
				resTime, val, err := s.refineExtremum(evalVal, times[i-1], times[i+1], true)
				if err != nil {
					return nil, err
				}

				// Validate if it is East or West based on RA difference.
				pos1, _ := spec.Target.Position(resTime)
				pos2, _ := spec.Other.Position(resTime)
				raDiff := pos1.RA().Degrees() - pos2.RA().Degrees()
				for raDiff > 180 {
					raDiff -= 360
				}
				for raDiff <= -180 {
					raDiff += 360
				}

				isEast := raDiff > 0

				if (spec.Kind == EventGreatestElongationEast && isEast) || (spec.Kind == EventGreatestElongationWest && !isEast) {
					events = append(events, Event{
						Kind:  spec.Kind,
						Time:  resTime,
						Value: val, // peak separation in degrees
					})
				}
			}
		}

		if spec.Kind == EventAppulse && i > 0 {
			// Find local minimum of angular separation
			v0 := vals[i-1]
			if v1 < v0 && v1 <= v2 {
				resTime, val, err := s.refineExtremum(evalVal, times[i-1], times[i+1], false)
				if err != nil {
					return nil, err
				}
				events = append(events, Event{
					Kind:  EventAppulse,
					Time:  resTime,
					Value: val, // minimum separation in degrees
				})
			}
		}
	}

	return events, nil
}

// Event represents a specific occurrence of a celestial target in the coord.
type Event struct {
	Kind              EventKind
	Time              time.Time
	Altitude          angle.Angle // Observed refracted altitude
	GeometricAltitude angle.Angle // True geometric altitude
	Azimuth           angle.Angle
	Observable        bool    // True if the event satisfies observation conditions (not used directly here)
	Value             float64 // Internal numeric value used by the solver (e.g. altitude difference)
}

func (e Event) String() string {
	return fmt.Sprintf("%-7s at %s (Alt: %s, Az: %s)", e.Kind, e.Time.ToGo().Format("2006-01-02 15:04:05"), e.Altitude, e.Azimuth)
}

// ── Twilight ──────────────────────────────────────────────────────────────────

// TwilightKind represents the level of agricultural/nautical/astronomical twilight.
type TwilightKind int

const (
	// CivilTwilight: Sun at -6° altitude.
	CivilTwilight TwilightKind = iota
	// NauticalTwilight: Sun at -12° altitude.
	NauticalTwilight
	// AstronomicalTwilight: Sun at -18° altitude.
	AstronomicalTwilight
)

func (k TwilightKind) String() string {
	switch k {
	case CivilTwilight:
		return "Civil"
	case NauticalTwilight:
		return "Nautical"
	case AstronomicalTwilight:
		return "Astronomical"
	default:
		return "Unknown"
	}
}

// TwilightThresholds maps each twilight kind to its solar altitude threshold (in degrees).
var TwilightThresholds = map[TwilightKind]float64{
	CivilTwilight:        -6.0,
	NauticalTwilight:     -12.0,
	AstronomicalTwilight: -18.0,
}

// TwilightEvent groups a dawn and dusk occurrence for a specific twilight level.
// If an event did not occur within the search interval, the pointer will be nil.
type TwilightEvent struct {
	Kind TwilightKind
	Dawn *Event
	Dusk *Event
}

// ── Event Finder API ──────────────────────────────────────────────────────────

// ── Sun/Moon/Twilight Helpers ──────────────────────────────────────────────────

// NOTE: Horizon altitude constants were removed in favor of SOFA-native
// refraction. Use Site.SunRiseSetThreshold(), Site.MoonRiseSetThreshold(),
// and Site.RiseSetThreshold() which account for SOFA's rigorous refraction
// model and only add body-specific corrections (semi-diameter, parallax).

// SunEvents returns all rise, set, and transit events for the Sun in the given interval.
// The threshold accounts for atmospheric refraction (34'), solar semi-diameter (16'),
// and geometric horizon dip from the site's elevation.
func SunEvents(start, end time.Time, site *Site, eph ephemeris.Provider) ([]Event, error) {
	sun := NewBody(ephemeris.Sun, eph)
	solver := NewEventSolver(15*time.Minute, 1*time.Second)
	spec := EventSpec{
		Family:    EventFamilyVisibility,
		Kind:      EventAnyVisibility,
		Target:    sun,
		Observer:  site,
		Threshold: site.SunRiseSetThreshold(),
	}
	return solver.Find(spec, start, end)
}

// SunriseSunset returns the first sunrise and first sunset found in the given interval.
// If an event is not found, the corresponding pointer will be nil.
func SunriseSunset(start, end time.Time, site *Site, eph ephemeris.Provider) (rise *Event, set *Event, err error) {
	events, err := SunEvents(start, end, site, eph)
	if err != nil {
		return nil, nil, err
	}

	for _, e := range events {
		if e.Kind == EventRise && rise == nil {
			ec := e
			rise = &ec
		}
		if e.Kind == EventSet && set == nil {
			ec := e
			set = &ec
		}
	}
	return rise, set, nil
}

// MoonEvents returns all rise, set, and transit events for the Moon in the given interval.
// The threshold accounts for atmospheric refraction, mean lunar semi-diameter,
// horizontal parallax, and geometric horizon dip from the site's elevation.
func MoonEvents(start, end time.Time, site *Site, eph ephemeris.Provider) ([]Event, error) {
	moon := NewBody(ephemeris.Moon, eph)
	solver := NewEventSolver(15*time.Minute, 1*time.Second)
	spec := EventSpec{
		Family:    EventFamilyVisibility,
		Kind:      EventAnyVisibility,
		Target:    moon,
		Observer:  site,
		Threshold: site.MoonRiseSetThreshold(),
	}
	return solver.Find(spec, start, end)
}

// MoonriseMoonset returns the first moonrise and first moonset found in the given interval.
// If an event is not found, the corresponding pointer will be nil.
func MoonriseMoonset(start, end time.Time, site *Site, eph ephemeris.Provider) (rise *Event, set *Event, err error) {
	events, err := MoonEvents(start, end, site, eph)
	if err != nil {
		return nil, nil, err
	}

	for _, e := range events {
		if e.Kind == EventRise && rise == nil {
			ec := e
			rise = &ec
		}
		if e.Kind == EventSet && set == nil {
			ec := e
			set = &ec
		}
	}
	return rise, set, nil
}

// TwilightEvents returns grouped dawn/dusk pairs for the given twilight kind and interval.
func TwilightEvents(start, end time.Time, site *Site, eph ephemeris.Provider, kind TwilightKind) ([]TwilightEvent, error) {
	threshold, ok := TwilightThresholds[kind]
	if !ok {
		return nil, nil
	}

	sun := NewBody(ephemeris.Sun, eph)
	solver := NewEventSolver(15*time.Minute, 1*time.Second)
	spec := EventSpec{
		Family:    EventFamilyVisibility,
		Kind:      EventAnyVisibility,
		Target:    sun,
		Observer:  site,
		Threshold: angle.Deg(threshold),
	}
	events, err := solver.Find(spec, start, end)
	if err != nil {
		return nil, err
	}

	var twilightEvents []TwilightEvent
	for i := 0; i < len(events); i++ {
		e := events[i]
		switch e.Kind {
		case EventRise:
			ec := e
			twilightEvents = append(twilightEvents, TwilightEvent{Kind: kind, Dawn: &ec})
		case EventSet:
			ec := e
			twilightEvents = append(twilightEvents, TwilightEvent{Kind: kind, Dusk: &ec})
		}
	}
	return twilightEvents, nil
}

// CivilDawnDusk returns the first civil dawn and first civil dusk found in the interval.
func CivilDawnDusk(start, end time.Time, site *Site, eph ephemeris.Provider) (dawn *Event, dusk *Event, err error) {
	return getTwilightPair(start, end, site, eph, CivilTwilight)
}

// NauticalDawnDusk returns the first nautical dawn and first nautical dusk found in the interval.
func NauticalDawnDusk(start, end time.Time, site *Site, eph ephemeris.Provider) (dawn *Event, dusk *Event, err error) {
	return getTwilightPair(start, end, site, eph, NauticalTwilight)
}

// AstronomicalDawnDusk returns the first astronomical dawn and first astronomical dusk found in the interval.
func AstronomicalDawnDusk(start, end time.Time, site *Site, eph ephemeris.Provider) (dawn *Event, dusk *Event, err error) {
	return getTwilightPair(start, end, site, eph, AstronomicalTwilight)
}

func getTwilightPair(start, end time.Time, site *Site, eph ephemeris.Provider, kind TwilightKind) (dawn *Event, dusk *Event, err error) {
	threshold := TwilightThresholds[kind]
	sun := NewBody(ephemeris.Sun, eph)
	solver := NewEventSolver(15*time.Minute, 1*time.Second)
	spec := EventSpec{
		Family:    EventFamilyVisibility,
		Kind:      EventAnyVisibility,
		Target:    sun,
		Observer:  site,
		Threshold: angle.Deg(threshold),
	}
	events, err := solver.Find(spec, start, end)
	if err != nil {
		return nil, nil, err
	}

	for _, e := range events {
		if e.Kind == EventRise && dawn == nil {
			ec := e
			dawn = &ec
		}
		if e.Kind == EventSet && dusk == nil {
			ec := e
			dusk = &ec
		}
	}
	return dawn, dusk, nil
}

// ── Geometry Helpers ──────────────────────────────────────────────────────────

// Conjunctions returns all conjunction events (same RA, ΔRA = 0) between target and other.
func Conjunctions(start, end time.Time, target, other Observable) ([]Event, error) {
	solver := NewEventSolver(6*time.Hour, 1*time.Second)
	spec := EventSpec{
		Family: EventFamilyRelativeGeometry,
		Kind:   EventConjunction,
		Target: target,
		Other:  other,
	}
	return solver.Find(spec, start, end)
}

// ConjunctionsEcliptic returns all ecliptic longitude conjunction events (Δλ = 0).
// This is the classical definition used in most historical astronomical literature.
func ConjunctionsEcliptic(start, end time.Time, target, other Observable) ([]Event, error) {
	solver := NewEventSolver(6*time.Hour, 1*time.Second)
	spec := EventSpec{
		Family: EventFamilyRelativeGeometry,
		Kind:   EventConjunctionEcliptic,
		Target: target,
		Other:  other,
	}
	return solver.Find(spec, start, end)
}

// Appulses returns all moments of minimum angular separation between target and other.
// The returned Event.Value contains the minimum separation in degrees.
func Appulses(start, end time.Time, target, other Observable) ([]Event, error) {
	solver := NewEventSolver(6*time.Hour, 1*time.Second)
	spec := EventSpec{
		Family: EventFamilyRelativeGeometry,
		Kind:   EventAppulse,
		Target: target,
		Other:  other,
	}
	return solver.Find(spec, start, end)
}

// Oppositions returns all opposition events between target and other in the given interval.
func Oppositions(start, end time.Time, target, other Observable) ([]Event, error) {
	solver := NewEventSolver(6*time.Hour, 1*time.Second)
	spec := EventSpec{
		Family: EventFamilyRelativeGeometry,
		Kind:   EventOpposition,
		Target: target,
		Other:  other,
	}
	return solver.Find(spec, start, end)
}

// GreatestElongations returns all Greatest Elongation events (both East and West) for a planet relative to the Sun.
func GreatestElongations(start, end time.Time, target, sun Observable) ([]Event, error) {
	solver := NewEventSolver(6*time.Hour, 1*time.Second)

	specEast := EventSpec{
		Family: EventFamilyRelativeGeometry,
		Kind:   EventGreatestElongationEast,
		Target: target,
		Other:  sun,
	}

	specWest := EventSpec{
		Family: EventFamilyRelativeGeometry,
		Kind:   EventGreatestElongationWest,
		Target: target,
		Other:  sun,
	}

	var allEvents []Event

	eastEvents, err := solver.Find(specEast, start, end)
	if err != nil {
		return nil, err
	}
	allEvents = append(allEvents, eastEvents...)

	westEvents, err := solver.Find(specWest, start, end)
	if err != nil {
		return nil, err
	}
	allEvents = append(allEvents, westEvents...)

	sort.Slice(allEvents, func(i, j int) bool {
		return allEvents[i].Time.Before(allEvents[j].Time)
	})

	return allEvents, nil
}

// FullMoonOppositions returns the full moons (Sun-Moon Oppositions) in the given interval.
// For eclipse detection, use LunarEclipses() which filters by ecliptic latitude.
func FullMoonOppositions(start, end time.Time, eph ephemeris.Provider) ([]Event, error) {
	sun := NewBody(ephemeris.Sun, eph)
	moon := NewBody(ephemeris.Moon, eph)

	// Moon moves very fast, so we use a higher resolution solver
	solver := NewEventSolver(6*time.Hour, 1*time.Second)
	spec := EventSpec{
		Family: EventFamilyRelativeGeometry,
		Kind:   EventOpposition,
		Target: moon,
		Other:  sun,
	}
	return solver.Find(spec, start, end)
}

// VisibilityEvents returns all rise, transit, and set events for a target at the given site.
// The rise/set threshold is automatically computed from the site's elevation, accounting
// for standard atmospheric refraction (34') and geometric horizon dip.
func VisibilityEvents(start, end time.Time, target Observable, site *Site) ([]Event, error) {
	solver := NewEventSolver(15*time.Minute, 1*time.Second)
	spec := EventSpec{
		Family:    EventFamilyVisibility,
		Kind:      EventAnyVisibility,
		Target:    target,
		Observer:  site,
		Threshold: site.RiseSetThreshold(),
	}
	return solver.Find(spec, start, end)
}

// ── Illumination Solver ──────────────────────────────────────────────────────

// EventAnyPhase is a wildcard to find all four lunar phases in a single pass.
const EventAnyPhase EventKind = -2

// solveIllumination finds lunar phase events (New Moon, First Quarter, Full Moon,
// Last Quarter) by tracking the geocentric elongation angle between the target
// (Moon) and the Sun using proper ecliptic longitude differences.
//
// This method delegates to moonElongation (phases.go) for rigorous ecliptic
// coordinate computation and wraps the results as Event types for the
// unified EventSolver framework.
func (s EventSolver) solveIllumination(spec EventSpec, start, end time.Time) ([]Event, error) {
	eph := ephemeris.Default()

	type phaseTarget struct {
		kind     EventKind
		elongDeg float64
	}

	var targets []phaseTarget
	switch spec.Kind {
	case EventNewMoon:
		targets = []phaseTarget{{EventNewMoon, 0}}
	case EventFirstQuarter:
		targets = []phaseTarget{{EventFirstQuarter, 90}}
	case EventFullMoon:
		targets = []phaseTarget{{EventFullMoon, 180}}
	case EventLastQuarter:
		targets = []phaseTarget{{EventLastQuarter, 270}}
	default:
		targets = []phaseTarget{
			{EventNewMoon, 0},
			{EventFirstQuarter, 90},
			{EventFullMoon, 180},
			{EventLastQuarter, 270},
		}
	}

	n := int(end.Sub(start)/s.Step) + 2
	times := make([]time.Time, 0, n)
	elongs := make([]float64, 0, n)

	for t := start; !t.After(end); t = t.Add(s.Step) {
		times = append(times, t)
		e, err := moonElongation(t, eph)
		if err != nil {
			return nil, err
		}
		elongs = append(elongs, e)
	}
	if last := times[len(times)-1]; last.Before(end) {
		times = append(times, end)
		e, err := moonElongation(end, eph)
		if err != nil {
			return nil, err
		}
		elongs = append(elongs, e)
	}

	var events []Event

	for _, pt := range targets {
		targetDeg := pt.elongDeg

		signedDist := func(elong float64) float64 {
			d := elong - targetDeg
			for d > 180 {
				d -= 360
			}
			for d <= -180 {
				d += 360
			}
			return d
		}

		evalDist := func(t time.Time) (float64, error) {
			e, err := moonElongation(t, eph)
			if err != nil {
				return 0, err
			}
			return signedDist(e), nil
		}

		for i := 0; i < len(times)-1; i++ {
			d1 := signedDist(elongs[i])
			d2 := signedDist(elongs[i+1])

			if (d1 <= 0 && d2 > 0) || (d1 > 0 && d2 <= 0) {
				if math.Abs(d1-d2) > 180 {
					continue
				}

				resTime, _, err := s.refineRoot(evalDist, times[i], times[i+1], d1)
				if err != nil {
					continue
				}

				e, _ := moonElongation(resTime, eph)
				illumination := (1.0 - math.Cos(e*math.Pi/180.0)) / 2.0

				events = append(events, Event{
					Kind:  pt.kind,
					Time:  resTime,
					Value: illumination,
				})
			}
		}
	}

	return events, nil
}

// NextNewMoon returns the first New Moon event after the given start time.
// Searches up to 35 days ahead (slightly more than one synodic month).
func NextNewMoon(start time.Time, eph ephemeris.Provider) (*Event, error) {
	end := start.AddDays(35)
	moon := NewBody(ephemeris.Moon, eph)
	solver := NewEventSolver(6*time.Hour, 1*time.Second)
	spec := EventSpec{
		Family: EventFamilyIllumination,
		Kind:   EventNewMoon,
		Target: moon,
	}
	events, err := solver.Find(spec, start, end)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, nil
	}
	return &events[0], nil
}

// NextFullMoon returns the first Full Moon event after the given start time.
// Searches up to 35 days ahead (slightly more than one synodic month).
func NextFullMoon(start time.Time, eph ephemeris.Provider) (*Event, error) {
	end := start.AddDays(35)
	moon := NewBody(ephemeris.Moon, eph)
	solver := NewEventSolver(6*time.Hour, 1*time.Second)
	spec := EventSpec{
		Family: EventFamilyIllumination,
		Kind:   EventFullMoon,
		Target: moon,
	}
	events, err := solver.Find(spec, start, end)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, nil
	}
	return &events[0], nil
}
