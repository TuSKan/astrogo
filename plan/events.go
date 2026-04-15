package plan

import (
	"fmt"
	"math"
	"sort"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
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

	// EventConjunction represents two targets having the same apparent longitude or right ascension.
	EventConjunction
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
	}
	return nil
}

func isPhaseEvent(k EventKind) bool {
	return false
}

// evaluator is a function that returns the metric to be solved (e.g. altitude diff, separation angle).
type evaluator func(t time.Time) (float64, error)

// EventSolver searches for astronomical events based on an EventSpec over a time interval.
type EventSolver struct {
	Step      time.Duration
	Tolerance time.Duration
	MaxIter   int
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
		Step:      step,
		Tolerance: tol,
		MaxIter:   50,
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

// refineRoot uses Brent's method to find the exact time when eval(t) == 0.
// Brent's method combines inverse quadratic interpolation, secant, and bisection
// for superlinear convergence while maintaining the robustness of bracketing.
// Requires: v1 and eval(t2) have opposite signs (bracketing condition).
func (s EventSolver) refineRoot(eval evaluator, t1, t2 time.Time, v1 float64) (time.Time, float64, error) {
	// Work in float64 seconds offset from t1 to avoid time.Duration precision issues.
	origin := t1
	tolSec := float64(s.Tolerance) / float64(time.Second)

	timeAt := func(sec float64) time.Time {
		return origin.Add(time.Duration(sec * float64(time.Second)))
	}

	a := 0.0
	b := float64(t2.Sub(t1)) / float64(time.Second)
	fa := v1
	fb, err := eval(timeAt(b))
	if err != nil {
		return time.Time{}, 0, err
	}

	c, fc := a, fa
	d := b - a
	e := d

	for i := 0; i < s.MaxIter; i++ {
		if math.Abs(b-a) < tolSec || fb == 0 {
			break
		}

		// Ensure |f(b)| <= |f(c)| for interpolation quality
		if (fb > 0 && fc > 0) || (fb < 0 && fc < 0) {
			c, fc = a, fa
			d = b - a
			e = d
		}

		if math.Abs(fc) < math.Abs(fb) {
			a, b, c = b, c, b
			fa, fb, fc = fb, fc, fb
		}

		tol1 := 2.0*1e-15*math.Abs(b) + 0.5*tolSec
		m := 0.5 * (c - b)

		if math.Abs(m) <= tol1 || fb == 0 {
			break
		}

		if math.Abs(e) >= tol1 && math.Abs(fa) > math.Abs(fb) {
			// Try interpolation
			sv := fb / fa
			var p, q float64

			if a == c {
				// Secant (linear)
				p = 2.0 * m * sv
				q = 1.0 - sv
			} else {
				// Inverse quadratic interpolation
				qv := fa / fc
				r := fb / fc
				p = sv * (2.0*m*qv*(qv-r) - (b-a)*(r-1.0))
				q = (qv - 1.0) * (r - 1.0) * (sv - 1.0)
			}

			if p > 0 {
				q = -q
			} else {
				p = -p
			}

			if 2.0*p < math.Min(3.0*m*q-math.Abs(tol1*q), math.Abs(e*q)) {
				e = d
				d = p / q
			} else {
				d = m
				e = d
			}
		} else {
			// Bisection
			d = m
			e = d
		}

		a = b
		fa = fb

		if math.Abs(d) > tol1 {
			b += d
		} else if m > 0 {
			b += tol1
		} else {
			b -= tol1
		}

		fb, err = eval(timeAt(b))
		if err != nil {
			return time.Time{}, 0, err
		}
	}

	resTime := timeAt(b)
	return resTime, fb, nil
}

// refineExtremum uses Brent's minimization to find the time of local maximum or minimum.
// Brent's minimization combines parabolic interpolation with golden section fallback
// for superlinear convergence on smooth functions.
func (s EventSolver) refineExtremum(eval evaluator, t1, t3 time.Time, isMax bool) (time.Time, float64, error) {
	const goldenRatio = 0.3819660112501051 // (3 - sqrt(5)) / 2

	a, b := t1, t3
	x := a.Add(time.Duration(float64(b.Sub(a)) * 0.5))

	fx, err := eval(x)
	if err != nil {
		return time.Time{}, 0, err
	}
	if isMax {
		fx = -fx
	}

	w, v := x, x
	fw, fv := fx, fx
	e := time.Duration(0) // Distance moved on the step before last
	d := time.Duration(0) // Distance moved on the last step

	for i := 0; i < s.MaxIter; i++ {
		midpoint := a.Add(time.Duration(float64(b.Sub(a)) * 0.5))
		tol1 := float64(s.Tolerance)
		tol2 := 2.0 * tol1

		if math.Abs(float64(x.Sub(midpoint)))+float64(b.Sub(a))/2.0 <= tol2 {
			break
		}

		useParabolic := false

		if math.Abs(float64(e)) > tol1 {
			// Fit parabola through x, v, w
			xw := float64(x.Sub(w))
			xv := float64(x.Sub(v))
			r := xw * (fx - fv)
			q := xv * (fx - fw)
			p := xv*q - xw*r
			q = 2.0 * (q - r)

			if q > 0 {
				p = -p
			} else {
				q = -q
			}

			// Accept parabolic step if it falls within the bracket and is a reduction
			if math.Abs(p) < math.Abs(0.5*q*float64(e)) &&
				p > q*float64(a.Sub(x)) &&
				p < q*float64(b.Sub(x)) {

				e = d
				d = time.Duration(p / q)
				useParabolic = true
			}
		}

		if !useParabolic {
			// Golden section step
			if x.After(midpoint) || x.Equal(midpoint) {
				e = a.Sub(x)
			} else {
				e = b.Sub(x)
			}
			d = time.Duration(float64(e) * goldenRatio)
		}

		// Evaluate at the new trial point
		var u time.Time
		if math.Abs(float64(d)) >= tol1 {
			u = x.Add(d)
		} else if float64(d) > 0 {
			u = x.Add(time.Duration(tol1))
		} else {
			u = x.Add(time.Duration(-tol1))
		}

		fu, err := eval(u)
		if err != nil {
			return time.Time{}, 0, err
		}
		if isMax {
			fu = -fu
		}

		// Update bracket
		if fu <= fx {
			if u.Before(x) {
				b = x
			} else {
				a = x
			}
			v, w, x = w, x, u
			fv, fw, fx = fw, fx, fu
		} else {
			if u.Before(x) {
				a = u
			} else {
				b = u
			}
			if fu <= fw || w.Equal(x) {
				v, w = w, u
				fv, fw = fw, fu
			} else if fu <= fv || v.Equal(x) || v.Equal(w) {
				v = u
				fv = fu
			}
		}
	}

	finalVal, err := eval(x)
	if err != nil {
		return time.Time{}, 0, err
	}
	return x, finalVal, err
}

func (s EventSolver) solveVisibility(spec EventSpec, start, end time.Time) ([]Event, error) {
	var events []Event

	evalVal := func(t time.Time) (float64, error) {
		pos, err := spec.Target.Position(t)
		if err != nil {
			return 0, err
		}

		ctx := coord.NewContext(t, spec.Observer.Location(), atmosphere.StandardAtmosphere)
		astro := coord.NewAstrometric(pos.RA(), pos.Dec())
		geom := ctx.AstrometricToObserved(astro)

		return geom.Alt().Degrees() - spec.Threshold.Degrees(), nil
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

				// Calculate geometric outputs
				resCtx := coord.NewContext(resTime, spec.Observer.Location(), atmosphere.StandardAtmosphere)
				pos, _ := spec.Target.Position(resTime)
				astro := coord.NewAstrometric(pos.RA(), pos.Dec())
				geom := resCtx.AstrometricToObserved(astro)
				aa, _ := resCtx.ICRSToAltAz(pos)

				events = append(events, Event{
					Kind:              kind,
					Time:              resTime,
					Altitude:          aa.Alt(),
					GeometricAltitude: geom.Alt(),
					Azimuth:           aa.Az(),
					Value:             geom.Alt().Degrees() - spec.Threshold.Degrees(),
				})
			}
		}

		// Local Maximum (Transit)
		if i > 0 {
			h0 := alts[i-1]
			if h1 > h0 && h1 >= h2 && (spec.Kind == EventTransit || spec.Kind == EventAnyVisibility) {
				resTime, _, err := s.refineExtremum(evalVal, times[i-1], times[i+1], true)
				if err != nil {
					return nil, err
				}

				resCtx := coord.NewContext(resTime, spec.Observer.Location(), atmosphere.StandardAtmosphere)
				pos, _ := spec.Target.Position(resTime)
				astro := coord.NewAstrometric(pos.RA(), pos.Dec())
				geom := resCtx.AstrometricToObserved(astro)
				aa, _ := resCtx.ICRSToAltAz(pos)

				events = append(events, Event{
					Kind:              EventTransit,
					Time:              resTime,
					Altitude:          aa.Alt(),
					GeometricAltitude: geom.Alt(),
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

		if spec.Kind == EventConjunction || spec.Kind == EventOpposition {
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

const (
	// SunHorizonAltitude is the standard altitude for sunrise/sunset (center of Sun).
	// It is -50 arcminutes (-0.8333°) to account for refraction (34') and
	// semi-diameter (16').
	SunHorizonAltitude = -0.8333

	// MoonHorizonAltitude is the default altitude for moonrise/moonset (center of Moon).
	// We use 0° by default as refraction vary and parallax is significant.
	MoonHorizonAltitude = 0.0
)

// SunEvents returns all rise, set, and transit events for the Sun in the given interval.
// It uses a threshold of -0.833° to account for atmospheric refraction and semi-diameter.
func SunEvents(start, end time.Time, site *Site, eph ephemeris.Provider) ([]Event, error) {
	sun := NewBody(ephemeris.Sun, eph)
	solver := NewEventSolver(15*time.Minute, 1*time.Second)
	spec := EventSpec{
		Family:    EventFamilyVisibility,
		Kind:      EventAnyVisibility,
		Target:    sun,
		Observer:  site,
		Threshold: angle.Deg(SunHorizonAltitude),
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
// It uses a threshold of 0° (center of the disk).
func MoonEvents(start, end time.Time, site *Site, eph ephemeris.Provider) ([]Event, error) {
	moon := NewBody(ephemeris.Moon, eph)
	solver := NewEventSolver(15*time.Minute, 1*time.Second)
	spec := EventSpec{
		Family:    EventFamilyVisibility,
		Kind:      EventAnyVisibility,
		Target:    moon,
		Observer:  site,
		Threshold: angle.Deg(MoonHorizonAltitude),
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

// Conjunctions returns all conjunction events between target and other in the given interval.
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

// LunarEclipses returns the full moons (Sun-Moon Oppositions) in the given interval
// which represent the syzygy alignment necessary for lunar eclipses.
func LunarEclipses(start, end time.Time, eph ephemeris.Provider) ([]Event, error) {
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

// VisibilityEvents returns all rise, transit, and set events for a target crossing the given threshold altitude within the interval.
func VisibilityEvents(start, end time.Time, target Observable, site *Site, threshold angle.Angle) ([]Event, error) {
	solver := NewEventSolver(15*time.Minute, 1*time.Second)
	spec := EventSpec{
		Family:    EventFamilyVisibility,
		Kind:      EventAnyVisibility,
		Target:    target,
		Observer:  site,
		Threshold: threshold,
	}
	return solver.Find(spec, start, end)
}
