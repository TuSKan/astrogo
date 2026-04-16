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

// ── Boundary Refinement Helpers ──────────────────────────────────────────────

// refineVisibility uses Chandrupatla root-finding to locate the precise time
// when a body's altitude crosses the threshold within [a, b].
//
// The altitude at a and b must bracket the threshold (one above, one below).
// Falls back to the grid point b if refinement fails.
func refineVisibility(
	obj coord.Object,
	site *Site,
	a, b time.Time,
	threshold angle.Angle,
) time.Time {
	altEval := func(t time.Time) (float64, error) {
		pos, err := obj.ICRS(t)
		if err != nil {
			return 0, err
		}
		ctx := coord.NewContext(t, site.Location(), site.Atmosphere())
		aa, err := ctx.ICRSToAltAz(pos)
		if err != nil {
			return 0, err
		}
		return aa.Alt().Degrees() - threshold.Degrees(), nil
	}
	solver := DefaultSolver()
	refined, _, err := solver.FindRoot(Evaluator(altEval), a, b)
	if err != nil {
		return b // fallback: use latest grid point
	}
	return refined
}

// refineBisect uses binary search to locate the precise time when a boolean
// state transition occurs within [a, b].
//
// check(a) must return aState, and check(b) must return !aState.
// After 20 bisections on a typical 5-minute bracket, precision is ~0.3 ms.
//
// This is used for constraint-based observability where the underlying
// function may be discontinuous (unlike altitude, which is continuous
// and uses Chandrupatla root-finding via refineVisibility).
func refineBisect(a, b time.Time, aState bool, check func(time.Time) bool) time.Time {
	const maxBisect = 20
	for i := 0; i < maxBisect; i++ {
		mid := a.Add(b.Sub(a) / 2)
		if check(mid) == aState {
			a = mid
		} else {
			b = mid
		}
	}
	return a.Add(b.Sub(a) / 2)
}

// ── Visibility Finders ───────────────────────────────────────────────────────

// VisibleIntervals finds contiguous time windows during which an object is
// above the specified altitude threshold.
//
// It uses a sampled grid search with the provided step size, then refines
// each boundary using Chandrupatla root-finding (sub-second precision).
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
	var prevT time.Time
	hasPrev := false

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
			// Transition: invisible → visible. Refine the exact crossing.
			if hasPrev {
				winStart = refineVisibility(obj, site, prevT, t, minAlt)
			} else {
				winStart = t
			}
			inWindow = true
		} else if !visible && inWindow {
			// Transition: visible → invisible. Refine the exact crossing.
			winEnd := refineVisibility(obj, site, prevT, t, minAlt)
			intervals = append(intervals, Interval{
				Object: obj,
				Window: Window{Start: winStart, End: winEnd},
			})
			inWindow = false
		}

		prevT = t
		hasPrev = true
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
//
// Transition boundaries are refined using binary search (sub-second precision).
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

	obs, ok := obj.(Observable)
	if !ok {
		return nil, errors.New("object does not implement Observable")
	}

	// Constraint check function for bisection refinement.
	checkObs := func(t time.Time) bool {
		for _, c := range constraints {
			res, err := c.Check(obs, t, site)
			if err != nil || !res.Pass {
				return false
			}
		}
		return true
	}

	intervals := make([]Interval, 0, 4)
	inWindow := false
	var winStart time.Time
	var prevT time.Time
	hasPrev := false
	prevOK := false

	t := start
	for t.Before(end) || t.Equal(end) {
		allOK := checkObs(t)

		if allOK && !inWindow {
			if hasPrev {
				winStart = refineBisect(prevT, t, prevOK, checkObs)
			} else {
				winStart = t
			}
			inWindow = true
		} else if !allOK && inWindow {
			winEnd := refineBisect(prevT, t, prevOK, checkObs)
			intervals = append(intervals, Interval{
				Object: obj,
				Window: Window{Start: winStart, End: winEnd},
			})
			inWindow = false
		}

		prevT = t
		prevOK = allOK
		hasPrev = true
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
