package plan

import (
	"errors"
	stdtime "time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"

	"github.com/TuSKan/astrogo/time"
)

// Interval is a continuous window during which an object is observable.
type Interval struct {
	Object coord.Object
	Window Window
}

// IsVisible returns true if the object is currently above the specified
// altitude threshold at the given site and time.
func IsVisible(obj coord.Object, t time.Time, site *Site, minAlt angle.Angle) (bool, error) {
	pos, err := obj.ICRS(t)
	if err != nil {
		return false, err
	}
	ctx := coord.NewContext(t, site.Location(), site.Atmosphere())
	aa, err := ctx.ICRSToAltAz(pos)
	if err != nil {
		return false, err
	}
	return aa.Alt().Degrees() >= minAlt.Degrees(), nil
}

// VisibleIntervals finds contiguous time windows during which an object is
// above the specified altitude threshold.
// It uses a sampled grid search with the provided step size.
func VisibleIntervals(
	obj coord.Object,
	site *Site,
	start, end time.Time,
	step stdtime.Duration,
	minAlt angle.Angle,
) ([]Interval, error) {
	if step <= 0 {
		step = 5 * stdtime.Minute
	}

	intervals := make([]Interval, 0, 4)
	inWindow := false
	var winStart time.Time

	t := start
	for t.Before(end) || t.Equal(end) {
		pos, err := obj.ICRS(t)
		if err != nil {
			return nil, err
		}

		ctx := coord.NewContext(t, site.Location(), site.Atmosphere())
		aa, err := ctx.ICRSToAltAz(pos)
		if err != nil {
			return nil, err
		}

		visible := aa.Alt().Degrees() >= minAlt.Degrees()

		if visible && !inWindow {
			winStart = t
			inWindow = true
		} else if !visible && inWindow {
			intervals = append(intervals, Interval{
				Object: obj,
				Window: Window{Start: winStart, End: t},
			})
			inWindow = false
		}
		t = t.Add(step)
	}

	if inWindow {
		intervals = append(intervals, Interval{
			Object: obj,
			Window: Window{Start: winStart, End: end},
		})
	}

	return intervals, nil
}

// TransitEstimate estimates the time and altitude of maximum culmination
// (transit) for an object within a given search window.
//
// It uses a two-stage approach:
//  1. Coarse 10-min grid scan to bracket the maximum.
//  2. Brent's minimization (via Solver) within the bracket for sub-second precision.
func TransitEstimate(obj coord.Object, site *Site, start, end time.Time) (time.Time, angle.Angle, error) {
	const coarseStep = 10 * stdtime.Minute

	// Stage 1: coarse scan to locate the bracket [tLeft, tRight] around the peak.
	type sample struct {
		t   time.Time
		alt float64
	}
	var samples []sample
	for t := start; !t.After(end); t = t.Add(coarseStep) {
		pos, err := obj.ICRS(t)
		if err != nil {
			return time.Time{}, angle.Deg(0), err
		}
		ctx := coord.NewContext(t, site.Location(), site.Atmosphere())
		aa, err := ctx.ICRSToAltAz(pos)
		if err != nil {
			return time.Time{}, angle.Deg(0), err
		}
		samples = append(samples, sample{t, aa.Alt().Degrees()})
	}
	if len(samples) == 0 {
		return time.Time{}, angle.Deg(0), nil
	}

	// Find index of maximum.
	maxIdx := 0
	for i, s := range samples {
		if s.alt > samples[maxIdx].alt {
			maxIdx = i
		}
	}

	// Stage 2: Brent's minimization on altitude within the surrounding bracket.
	a := samples[max(0, maxIdx-1)].t
	b := samples[min(len(samples)-1, maxIdx+1)].t

	altAt := func(t time.Time) (float64, error) {
		pos, err := obj.ICRS(t)
		if err != nil {
			return 0, err
		}
		ctx := coord.NewContext(t, site.Location(), site.Atmosphere())
		aa, err := ctx.ICRSToAltAz(pos)
		if err != nil {
			return 0, err
		}
		return aa.Alt().Degrees(), nil
	}

	solver := DefaultSolver()
	resTime, _, err := solver.FindExtremum(Evaluator(altAt), a, b, true)
	if err != nil {
		return time.Time{}, angle.Deg(0), err
	}

	pos, err := obj.ICRS(resTime)
	if err != nil {
		return time.Time{}, angle.Deg(0), err
	}
	resCtx := coord.NewContext(resTime, site.Location(), site.Atmosphere())
	aa, err := resCtx.ICRSToAltAz(pos)
	if err != nil {
		return time.Time{}, angle.Deg(0), err
	}
	return resTime, aa.Alt(), nil
}

// MaxAltitudeInWindow returns the maximum altitude reached by an object
// in the specified time window.
func MaxAltitudeInWindow(obj coord.Object, site *Site, start, end time.Time) (angle.Angle, error) {
	_, alt, err := TransitEstimate(obj, site, start, end)
	return alt, err
}

// Find scans [start, end] in steps of step, returning all intervals during
// which obj satisfies all constraints from site.
func Find(
	obj coord.Object,
	site *Site,
	constraints []Constraint,
	start, end time.Time,
	step stdtime.Duration,
) ([]Interval, error) {
	if step <= 0 {
		step = 5 * stdtime.Minute
	}

	intervals := make([]Interval, 0, 4)
	inWindow := false
	var winStart time.Time

	t := start
	for t.Before(end) || t.Equal(end) {
		// Adapt coord.Object to Observable if needed,
		// but since Observable is simpler, it should work if we cast or if we change the signature.
		// For now, let's just use a local adapter if needed.

		obs, ok := obj.(Observable)
		if !ok {
			// If it's not a Observable, we can't check constraints that require it.
			return nil, errors.New("object does not implement Observable")
		}

		allOK := true
		for _, c := range constraints {
			res, err := c.Check(obs, t, site)
			if err != nil {
				return nil, err
			}
			if !res.Pass {
				allOK = false
				break
			}
		}

		if allOK && !inWindow {
			winStart = t
			inWindow = true
		} else if !allOK && inWindow {
			intervals = append(intervals, Interval{
				Object: obj,
				Window: Window{Start: winStart, End: t},
			})
			inWindow = false
		}
		t = t.Add(step)
	}

	if inWindow {
		intervals = append(intervals, Interval{
			Object: obj,
			Window: Window{Start: winStart, End: end},
		})
	}

	return intervals, nil
}
