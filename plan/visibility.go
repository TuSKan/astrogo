package plan

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"

	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
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
		return false, fmt.Errorf("visibility: ICRS: %w", err)
	}

	ctx := coord.NewContext(t, site.Location(), site.Refraction())

	aa, err := observedAltAz(obj, t, ctx, pos)
	if err != nil {
		return false, fmt.Errorf("visibility: AltAz: %w", err)
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
	ctxAt func(time.Time) *coord.Context,
	a, b time.Time,
	threshold angle.Angle,
) time.Time {
	altEval := func(t time.Time) (float64, error) {
		pos, err := obj.ICRS(t)
		if err != nil {
			return 0, fmt.Errorf("visibility: ICRS: %w", err)
		}

		aa, err := observedAltAz(obj, t, ctxAt(t), pos)
		if err != nil {
			return 0, fmt.Errorf("visibility: AltAz: %w", err)
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
func refineBisect(a, b time.Time, aState bool, check func(time.Time) (bool, error)) (time.Time, error) {
	const maxBisect = 20

	for range maxBisect {
		mid := a.Add(b.Sub(a) / 2)

		state, err := check(mid)
		if err != nil {
			return time.Time{}, err
		}

		if state == aState {
			a = mid
		} else {
			b = mid
		}
	}

	return a.Add(b.Sub(a) / 2), nil
}

// ── Visibility Finders ───────────────────────────────────────────────────────

// VisibleIntervals finds contiguous time windows during which an object is
// above the specified altitude threshold.
//
// It uses a sampled grid search with the provided step size, then refines
// each boundary using Chandrupatla root-finding (sub-second precision).
//
// The step must be positive and at most 15 minutes. Larger steps risk missing
// short visibility windows entirely and will return an error.
func VisibleIntervals(
	obj coord.Object,
	site *Site,
	start, end time.Time,
	step time.Duration,
	minAlt angle.Angle,
) ([]Interval, error) {
	return visibleIntervals(obj, movingContext(site.Location(), site.Refraction()), start, end, step, minAlt)
}

// nextSample returns the sample after t on a scan ending at end, in steps of
// step, with the last step cut short so the scan's final sample is end
// itself; more is false once end has been sampled.
//
// The window finders used to step while t <= end, which never evaluates end
// unless the steps happen to land on it. A target setting after the last
// sample was then reported up until end, and one rising after it was missed
// altogether (#550).
func nextSample(t, end time.Time, step unit.Duration) (next time.Time, more bool) {
	if !t.Before(end) {
		return t, false
	}

	next = t.Add(step)
	if next.After(end) {
		next = end
	}

	return next, true
}

// visibleIntervals is VisibleIntervals with the Context for each instant
// supplied by ctxAt.
//
// That used to be a full coord.NewContext per sample and per refinement
// iteration, which was 98% of the function's time; VisibleIntervals passes a
// movingContext, whose bound is coord.Context.SetTime's (#480). The parameter
// is what lets a test run this same algorithm on full Contexts as its
// reference.
func visibleIntervals(
	obj coord.Object,
	ctxAt func(time.Time) *coord.Context,
	start, end time.Time,
	step time.Duration,
	minAlt angle.Angle,
) ([]Interval, error) {
	if step <= 0 {
		step = 5 * time.Minute
	}

	if step > 15*time.Minute {
		return nil, fmt.Errorf("%w: %v", ErrStepTooLarge, step)
	}

	intervals := make([]Interval, 0, 4)
	inWindow := false

	var (
		winStart time.Time
		prevT    time.Time
	)

	hasPrev := false

	for t, more := start, !start.After(end); more; t, more = nextSample(t, end, time.FromGoDuration(step)) {
		pos, err := obj.ICRS(t)
		if err != nil {
			return nil, fmt.Errorf("visibility: ICRS: %w", err)
		}

		aa, err := observedAltAz(obj, t, ctxAt(t), pos)
		if err != nil {
			return nil, fmt.Errorf("visibility: AltAz: %w", err)
		}

		visible := aa.Alt().Degrees() >= minAlt.Degrees()

		if visible && !inWindow {
			// Transition: invisible → visible. Refine the exact crossing.
			if hasPrev {
				winStart = refineVisibility(obj, ctxAt, prevT, t, minAlt)
			} else {
				winStart = t
			}

			inWindow = true
		} else if !visible && inWindow {
			// Transition: visible → invisible. Refine the exact crossing.
			winEnd := refineVisibility(obj, ctxAt, prevT, t, minAlt)
			intervals = append(intervals, Interval{
				Object: obj,
				Window: Window{Start: winStart, End: winEnd},
			})
			inWindow = false
		}

		prevT = t
		hasPrev = true
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
	return transitEstimate(obj, site, movingContext(site.Location(), site.Refraction()), start, end)
}

// transitEstimate is TransitEstimate with the Context for each scan sample and
// solver iteration supplied by ctxAt, for the reason visibleIntervals gives.
// The altitude it returns is still read through a full Context at the refined
// instant, so the reported culmination carries no SetTime approximation.
func transitEstimate(
	obj coord.Object,
	site *Site,
	ctxAt func(time.Time) *coord.Context,
	start, end time.Time,
) (time.Time, angle.Angle, error) {
	if err := checkInterval(start, end); err != nil {
		return time.Time{}, angle.Deg(0), err
	}

	coarseStep := unit.Minutes(10)

	// Stage 1: coarse scan to locate the bracket [tLeft, tRight] around the peak.
	type sample struct {
		t   time.Time
		alt float64
	}

	samples := make([]sample, 0, int(end.Sub(start)/coarseStep)+2)

	sampleAt := func(t time.Time) error {
		pos, err := obj.ICRS(t)
		if err != nil {
			return fmt.Errorf("visibility: transit ICRS: %w", err)
		}

		aa, err := observedAltAz(obj, t, ctxAt(t), pos)
		if err != nil {
			return err
		}

		samples = append(samples, sample{t, aa.Alt().Degrees()})

		return nil
	}

	for t := start; !t.After(end); t = t.Add(coarseStep) {
		if err := sampleAt(t); err != nil {
			return time.Time{}, angle.Deg(0), err
		}
	}

	// The window's end, whenever the steps did not land on it. Until #540 a
	// window that was not a whole number of steps never evaluated it, so a
	// target still rising at the end — the common case for a window that
	// closes at dawn — was reported at the last step before, up to 10 min
	// early and 2.3° low. A window shorter than one step sampled only start.
	if samples[len(samples)-1].t.Before(end) {
		if err := sampleAt(end); err != nil {
			return time.Time{}, angle.Deg(0), err
		}
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
			return 0, fmt.Errorf("visibility: transit ICRS: %w", err)
		}

		aa, err := observedAltAz(obj, t, ctxAt(t), pos)
		if err != nil {
			return 0, fmt.Errorf("visibility: transit AltAz: %w", err)
		}

		return aa.Alt().Degrees(), nil
	}

	solver := DefaultSolver()

	resTime, _, err := solver.FindExtremum(Evaluator(altAt), a, b, true)
	if err != nil {
		return time.Time{}, angle.Deg(0), err
	}

	fullAlt := func(t time.Time) (angle.Angle, error) {
		pos, err := obj.ICRS(t)
		if err != nil {
			return angle.Deg(0), err
		}

		aa, err := observedAltAz(obj, t, coord.NewContext(t, site.Location(), site.Refraction()), pos)
		if err != nil {
			return angle.Deg(0), err
		}

		return aa.Alt(), nil
	}

	resAlt, err := fullAlt(resTime)
	if err != nil {
		return time.Time{}, angle.Deg(0), err
	}

	// Brent's method never evaluates the ends of its bracket, so when the
	// highest sample is an edge of the window — a target rising through
	// the whole window, or setting through it — the maximum is that edge
	// and Brent only approaches it to within its tolerance. The edge is
	// read through the same full Context as the refined instant, so the
	// two altitudes compare like for like.
	var edges []time.Time
	if maxIdx == 0 {
		edges = append(edges, start)
	}

	if maxIdx == len(samples)-1 {
		edges = append(edges, end)
	}

	for _, edge := range edges {
		edgeAlt, err := fullAlt(edge)
		if err != nil {
			return time.Time{}, angle.Deg(0), err
		}

		if edgeAlt > resAlt {
			resTime, resAlt = edge, edgeAlt
		}
	}

	return resTime, resAlt, nil
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
//
// The step must be positive and at most 15 minutes. Larger steps risk missing
// short visibility windows entirely and will return an error.
func Find(
	obj coord.Object,
	site *Site,
	constraints []Constraint,
	start, end time.Time,
	step time.Duration,
) ([]Interval, error) {
	return find(obj, site, movingContext(site.Location(), site.Refraction()), constraints, start, end, step)
}

// find is Find with the Context for each instant supplied by ctxAt, for the
// reason visibleIntervals gives.
//
// It also shares that Context across the constraints, through ConstraintCtx,
// as IsObservable and the scheduler do. Find called Check, and every built-in
// Check builds its own full Context, so a sample cost one Apco13 per
// constraint rather than one.
func find(
	obj coord.Object,
	site *Site,
	ctxAt func(time.Time) *coord.Context,
	constraints []Constraint,
	start, end time.Time,
	step time.Duration,
) ([]Interval, error) {
	if err := checkInterval(start, end); err != nil {
		return nil, err
	}

	if step <= 0 {
		step = 5 * time.Minute
	}

	if step > 15*time.Minute {
		return nil, fmt.Errorf("%w: %v", ErrStepTooLarge, step)
	}

	obs, ok := obj.(Observable)
	if !ok {
		return nil, ErrNotObservable
	}

	// Constraint check function for bisection refinement.
	checkObs := func(t time.Time) (bool, error) {
		ctx := ctxAt(t)

		for _, c := range constraints {
			var (
				res Result
				err error
			)
			if cc, ok := c.(ConstraintCtx); ok {
				res, err = cc.CheckCtx(obs, t, site, ctx)
			} else {
				res, err = c.Check(obs, t, site)
			}

			if err != nil {
				return false, fmt.Errorf("plan: constraint at %s: %w",
					t.Format(time.RFC3339), err)
			}

			if !res.Pass {
				return false, nil
			}
		}

		return true, nil
	}

	intervals := make([]Interval, 0, 4)
	inWindow := false

	var (
		winStart time.Time
		prevT    time.Time
	)

	hasPrev := false
	prevOK := false

	for t, more := start, !start.After(end); more; t, more = nextSample(t, end, time.FromGoDuration(step)) {
		allOK, err := checkObs(t)
		if err != nil {
			return nil, err
		}

		if allOK && !inWindow {
			if hasPrev {
				winStart, err = refineBisect(prevT, t, prevOK, checkObs)
				if err != nil {
					return nil, err
				}
			} else {
				winStart = t
			}

			inWindow = true
		} else if !allOK && inWindow {
			winEnd, err := refineBisect(prevT, t, prevOK, checkObs)
			if err != nil {
				return nil, err
			}

			intervals = append(intervals, Interval{
				Object: obj,
				Window: Window{Start: winStart, End: winEnd},
			})
			inWindow = false
		}

		prevT = t
		prevOK = allOK
		hasPrev = true
	}

	if inWindow {
		intervals = append(intervals, Interval{
			Object: obj,
			Window: Window{Start: winStart, End: end},
		})
	}

	return intervals, nil
}
