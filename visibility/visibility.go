package visibility

import (
	"errors"
	"math"
	stdtime "time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constraint"
	"github.com/TuSKan/astrogo/observatory"
	"github.com/TuSKan/astrogo/sky"
	"github.com/TuSKan/astrogo/target"
	"github.com/TuSKan/astrogo/time"
)

// Window is a contiguous time interval.
type Window struct {
	Start time.Time
	End   time.Time
}

// Duration returns the duration of the window as a standard time.Duration.
func (w Window) Duration() stdtime.Duration {
	return w.End.Sub(w.Start)
}

// Interval is a continuous window during which an object is observable.
type Interval struct {
	Object sky.Object
	Window Window
}

// IsVisible returns true if the object is currently above the specified
// altitude threshold at the given site and time.
func IsVisible(obj sky.Object, t time.Time, site observatory.Site, minAlt angle.Angle) (bool, error) {
	pos, err := obj.ICRS(t)
	if err != nil {
		return false, err
	}
	aa, err := sky.AltAz(pos, t, site)
	if err != nil {
		return false, err
	}
	return aa.Alt.Degrees() >= minAlt.Degrees(), nil
}

// VisibleIntervals finds contiguous time windows during which an object is
// above the specified altitude threshold.
// It uses a sampled grid search with the provided step size.
func VisibleIntervals(
	obj sky.Object,
	site observatory.Site,
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
		// For efficiency in v1 we re-calculate position at each step.
		// For fixed stars (ICRS) this is redundant, for planets it is required.
		pos, err := obj.ICRS(t)
		if err != nil {
			return nil, err
		}

		aa, err := sky.AltAz(pos, t, site)
		if err != nil {
			return nil, err
		}

		visible := aa.Alt.Degrees() >= minAlt.Degrees()

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
//  2. Golden-section search within the bracket for sub-minute precision.
func TransitEstimate(obj sky.Object, site observatory.Site, start, end time.Time) (time.Time, angle.Angle, error) {
	const coarseStep = 10 * stdtime.Minute
	const tol = 1 * stdtime.Second

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
		aa, err := sky.AltAz(pos, t, site)
		if err != nil {
			return time.Time{}, angle.Deg(0), err
		}
		samples = append(samples, sample{t, aa.Alt.Degrees()})
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

	// Stage 2: golden-section search within the surrounding bracket.
	lo := samples[max(0, maxIdx-1)].t
	hi := samples[min(len(samples)-1, maxIdx+1)].t

	R := (math.Sqrt(5) - 1) / 2
	C := 1 - R

	altAt := func(t time.Time) (float64, error) {
		pos, err := obj.ICRS(t)
		if err != nil {
			return 0, err
		}
		aa, err := sky.AltAz(pos, t, site)
		if err != nil {
			return 0, err
		}
		return aa.Alt.Degrees(), nil
	}

	d := hi.Sub(lo)
	ga := lo.Add(stdtime.Duration(float64(d) * C))
	gb := lo.Add(stdtime.Duration(float64(d) * R))
	fa, err := altAt(ga)
	if err != nil {
		return time.Time{}, angle.Deg(0), err
	}
	fb, err := altAt(gb)
	if err != nil {
		return time.Time{}, angle.Deg(0), err
	}

	for hi.Sub(lo) > tol {
		if fa > fb {
			hi = gb
			gb, fb = ga, fa
			d = hi.Sub(lo)
			ga = lo.Add(stdtime.Duration(float64(d) * C))
			if fa, err = altAt(ga); err != nil {
				return time.Time{}, angle.Deg(0), err
			}
		} else {
			lo = ga
			ga, fa = gb, fb
			d = hi.Sub(lo)
			gb = lo.Add(stdtime.Duration(float64(d) * R))
			if fb, err = altAt(gb); err != nil {
				return time.Time{}, angle.Deg(0), err
			}
		}
	}

	var resTime time.Time
	if fa > fb {
		resTime = ga
	} else {
		resTime = gb
	}

	pos, err := obj.ICRS(resTime)
	if err != nil {
		return time.Time{}, angle.Deg(0), err
	}
	aa, err := sky.AltAz(pos, resTime, site)
	if err != nil {
		return time.Time{}, angle.Deg(0), err
	}
	return resTime, aa.Alt, nil
}

// MaxAltitudeInWindow returns the maximum altitude reached by an object
// in the specified time window.
func MaxAltitudeInWindow(obj sky.Object, site observatory.Site, start, end time.Time) (angle.Angle, error) {
	_, alt, err := TransitEstimate(obj, site, start, end)
	return alt, err
}

// Find scans [start, end] in steps of step, returning all intervals during
// which obj satisfies all constraints from site.
func Find(
	obj sky.Object,
	site observatory.Site,
	constraints []constraint.Constraint,
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
		// Adapt sky.Object to target.Observable if needed,
		// but since target.Observable is simpler, it should work if we cast or if we change the signature.
		// For now, let's just use a local adapter if needed.

		obs, ok := obj.(target.Observable)
		if !ok {
			// If it's not a target.Observable, we can't check constraints that require it.
			return nil, errors.New("object does not implement target.Observable")
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
