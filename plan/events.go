package plan

import (
	"fmt"
	"math"
	"sort"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/body"
	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/observatory"
	"github.com/TuSKan/astrogo/sky"
	"github.com/TuSKan/astrogo/target"
	"github.com/TuSKan/astrogo/time"
)

// EventKind defines the type of astronomical event.
type EventKind int

const (
	// EventRise represents the target crossing upward through the threshold.
	EventRise EventKind = iota
	// EventSet represents the target crossing downward through the threshold.
	EventSet
	// EventTransit represents the target reaching its local maximum altitude.
	EventTransit
)

func (k EventKind) String() string {
	switch k {
	case EventRise:
		return "Rise"
	case EventSet:
		return "Set"
	case EventTransit:
		return "Transit"
	default:
		return "Unknown"
	}
}

// Event represents a specific occurrence of a celestial target in the sky.
type Event struct {
	Kind       EventKind
	Time       time.Time
	Altitude   angle.Angle
	Azimuth    angle.Angle
	Observable bool    // True if the event satisfies observation conditions (not used directly here)
	Value      float64 // Internal numeric value used by the solver (e.g. altitude difference)
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

// EventFinder searches for rise, set, and transit events over a time interval.
type EventFinder struct {
	// Step is the coarse sampling interval for bracketing events.
	Step time.Duration
	// Tolerance is the desired precision for the event time.
	Tolerance time.Duration
	// MaxIter is the maximum number of iterations for the numerical solver.
	MaxIter int
}

// NewEventFinder creates a new EventFinder with the given step and tolerance.
// A step of 15-30 minutes and tolerance of 1 second is typically sufficient for most uses.
func NewEventFinder(step, tol time.Duration) EventFinder {
	return EventFinder{
		Step:      step,
		Tolerance: tol,
		MaxIter:   50, // Generally enough for bisection/golden-section to converge
	}
}

// FindEvents locates all rise, set, and transit events for the given object
// within the [start, end] interval relative to the observer's site and threshold altitude.
func (f EventFinder) FindEvents(
	obj target.Observable,
	start, end time.Time,
	site observatory.Site,
	threshold angle.Angle,
) ([]Event, error) {
	if f.Step <= 0 {
		f.Step = 15 * time.Minute
	}
	if f.Tolerance <= 0 {
		f.Tolerance = 1 * time.Second
	}
	if f.MaxIter <= 0 {
		f.MaxIter = 50
	}

	var events []Event

	// Pre-sample the interval to find brackets
	// We want to ensure we don't miss narrow peaks, so we use a reasonable step.
	// 15-30 mins is usually fine for celestial motion.
	n := int(end.Sub(start)/f.Step) + 2
	times := make([]time.Time, 0, n)
	alts := make([]float64, 0, n)

	for t := start; !t.After(end); t = t.Add(f.Step) {
		times = append(times, t)
		h, err := f.altitudeDiff(obj, t, site, threshold)
		if err != nil {
			return nil, err
		}
		alts = append(alts, h)
	}
	// Ensure the end point is included
	if last := times[len(times)-1]; last.Before(end) {
		times = append(times, end)
		h, err := f.altitudeDiff(obj, end, site, threshold)
		if err != nil {
			return nil, err
		}
		alts = append(alts, h)
	}

	for i := 0; i < len(times)-1; i++ {
		t1, t2 := times[i], times[i+1]
		h1, h2 := alts[i], alts[i+1]

		// 1. Crossing (Rise/Set)
		// We look for sign changes in altitude - threshold.
		if (h1 <= 0 && h2 > 0) || (h1 > 0 && h2 <= 0) {
			kind := EventRise
			if h1 > 0 {
				kind = EventSet
			}
			event, err := f.refineRoot(obj, t1, t2, h1, h2, site, threshold, kind)
			if err != nil {
				return nil, err
			}
			events = append(events, event)
		}

		// 2. Local Maximum (Transit)
		// We need three points to bracket a maximum: i-1, i, i+1.
		// If alts[i] is greater than both neighbors, there's a peak.
		if i > 0 {
			h0 := alts[i-1]
			if h1 > h0 && h1 >= h2 {
				// Peak is bracketed by [times[i-1], times[i+1]]
				event, err := f.refineMax(obj, times[i-1], times[i+1], site, threshold)
				if err != nil {
					return nil, err
				}
				events = append(events, event)
			}
		}
	}

	// Sort events by time
	sort.Slice(events, func(i, j int) bool {
		return events[i].Time.Before(events[j].Time)
	})

	return events, nil
}

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
func SunEvents(start, end time.Time, site observatory.Site, eph ephemeris.Provider) ([]Event, error) {
	sun := target.NewBody(body.Sun, eph)
	finder := NewEventFinder(15*time.Minute, 1*time.Second)
	return finder.FindEvents(sun, start, end, site, angle.Deg(SunHorizonAltitude))
}

// SunriseSunset returns the first sunrise and first sunset found in the given interval.
// If an event is not found, the corresponding pointer will be nil.
func SunriseSunset(start, end time.Time, site observatory.Site, eph ephemeris.Provider) (rise *Event, set *Event, err error) {
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
func MoonEvents(start, end time.Time, site observatory.Site, eph ephemeris.Provider) ([]Event, error) {
	moon := target.NewBody(body.Moon, eph)
	finder := NewEventFinder(15*time.Minute, 1*time.Second)
	return finder.FindEvents(moon, start, end, site, angle.Deg(MoonHorizonAltitude))
}

// MoonriseMoonset returns the first moonrise and first moonset found in the given interval.
// If an event is not found, the corresponding pointer will be nil.
func MoonriseMoonset(start, end time.Time, site observatory.Site, eph ephemeris.Provider) (rise *Event, set *Event, err error) {
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
func TwilightEvents(start, end time.Time, site observatory.Site, eph ephemeris.Provider, kind TwilightKind) ([]TwilightEvent, error) {
	threshold, ok := TwilightThresholds[kind]
	if !ok {
		return nil, nil
	}

	sun := target.NewBody(body.Sun, eph)
	finder := NewEventFinder(15*time.Minute, 1*time.Second)
	events, err := finder.FindEvents(sun, start, end, site, angle.Deg(threshold))
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
func CivilDawnDusk(start, end time.Time, site observatory.Site, eph ephemeris.Provider) (dawn *Event, dusk *Event, err error) {
	return getTwilightPair(start, end, site, eph, CivilTwilight)
}

// NauticalDawnDusk returns the first nautical dawn and first nautical dusk found in the interval.
func NauticalDawnDusk(start, end time.Time, site observatory.Site, eph ephemeris.Provider) (dawn *Event, dusk *Event, err error) {
	return getTwilightPair(start, end, site, eph, NauticalTwilight)
}

// AstronomicalDawnDusk returns the first astronomical dawn and first astronomical dusk found in the interval.
func AstronomicalDawnDusk(start, end time.Time, site observatory.Site, eph ephemeris.Provider) (dawn *Event, dusk *Event, err error) {
	return getTwilightPair(start, end, site, eph, AstronomicalTwilight)
}

func getTwilightPair(start, end time.Time, site observatory.Site, eph ephemeris.Provider, kind TwilightKind) (dawn *Event, dusk *Event, err error) {
	threshold := TwilightThresholds[kind]
	sun := target.NewBody(body.Sun, eph)
	finder := NewEventFinder(15*time.Minute, 1*time.Second)
	events, err := finder.FindEvents(sun, start, end, site, angle.Deg(threshold))
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

// altitudeDiff returns alt(t) - threshold in degrees.
func (f EventFinder) altitudeDiff(
	obj target.Observable,
	t time.Time,
	site observatory.Site,
	threshold angle.Angle,
) (float64, error) {
	pos, err := obj.Position(t)
	if err != nil {
		return 0, err
	}
	aa, err := sky.AltAz(pos, t, site)
	if err != nil {
		return 0, err
	}
	return aa.Alt.Degrees() - threshold.Degrees(), nil
}

// ── Numerical Solver ──────────────────────────────────────────────────────────

// refineRoot uses bisection to find the exact time when altitudeDiff == 0.
// This is used for finding precise rise and set times.
func (f EventFinder) refineRoot(
	obj target.Observable,
	t1, t2 time.Time,
	h1, h2 float64,
	site observatory.Site,
	threshold angle.Angle,
	kind EventKind,
) (Event, error) {
	low, high := t1, t2
	f1 := h1

	for i := 0; i < f.MaxIter; i++ {
		if high.Sub(low) < f.Tolerance {
			break
		}

		mid := low.Add(high.Sub(low) / 2)
		fm, err := f.altitudeDiff(obj, mid, site, threshold)
		if err != nil {
			return Event{}, err
		}

		if (f1 > 0) == (fm > 0) {
			low = mid
			f1 = fm
		} else {
			high = mid
		}
	}

	resTime := low.Add(high.Sub(low) / 2)
	pos, err := obj.Position(resTime)
	if err != nil {
		return Event{}, err
	}
	aa, err := sky.AltAz(pos, resTime, site)
	if err != nil {
		return Event{}, err
	}

	return Event{
		Kind:     kind,
		Time:     resTime,
		Altitude: aa.Alt,
		Azimuth:  aa.Az,
		Value:    aa.Alt.Degrees() - threshold.Degrees(),
	}, nil
}

// refineMax uses golden section search to find the time of maximum altitude.
func (f EventFinder) refineMax(
	obj target.Observable,
	t1, t3 time.Time,
	site observatory.Site,
	threshold angle.Angle,
) (Event, error) {
	// Golden section search for maximum in [t1, t3]
	// Using R = (sqrt(5)-1)/2
	R := (math.Sqrt(5) - 1) / 2
	C := 1 - R

	low, high := t1, t3
	d := high.Sub(low)

	ga := low.Add(time.Duration(float64(d) * C))
	gb := low.Add(time.Duration(float64(d) * R))

	fa, err := f.altitudeDiff(obj, ga, site, threshold)
	if err != nil {
		return Event{}, err
	}
	fb, err := f.altitudeDiff(obj, gb, site, threshold)
	if err != nil {
		return Event{}, err
	}

	for i := 0; i < f.MaxIter; i++ {
		if high.Sub(low) < f.Tolerance {
			break
		}

		if fa > fb {
			high = gb
			gb = ga
			fb = fa
			d = high.Sub(low)
			ga = low.Add(time.Duration(float64(d) * C))
			fa, err = f.altitudeDiff(obj, ga, site, threshold)
			if err != nil {
				return Event{}, err
			}
		} else {
			low = ga
			ga = gb
			fa = fb
			d = high.Sub(low)
			gb = low.Add(time.Duration(float64(d) * R))
			fb, err = f.altitudeDiff(obj, gb, site, threshold)
			if err != nil {
				return Event{}, err
			}
		}
	}

	var resTime time.Time
	if fa > fb {
		resTime = ga
	} else {
		resTime = gb
	}

	pos, err := obj.Position(resTime)
	if err != nil {
		return Event{}, err
	}
	aa, err := sky.AltAz(pos, resTime, site)
	if err != nil {
		return Event{}, err
	}

	return Event{
		Kind:     EventTransit,
		Time:     resTime,
		Altitude: aa.Alt,
		Azimuth:  aa.Az,
		Value:    aa.Alt.Degrees(),
	}, nil
}
