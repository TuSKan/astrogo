package plan

import (
	"fmt"
	"sort"

	"github.com/TuSKan/astrogo/coord"

	"github.com/TuSKan/astrogo/time"
)

// Observation pairs a Target with a specific observing time requirement.
type Observation struct {
	Target   Observable
	Duration time.Duration
}

// Slot pairs a coord.Object with an observing Window.
type Slot struct {
	Object Observable
	Window Window
}

// Planner evaluates coord.Objects against a set of Constraints at a given Site.
type Planner struct {
	Site        *Site
	Constraints []Constraint
}

// NewPlanner creates a new Planner for the given site and constraints.
func NewPlanner(site *Site, constraints []Constraint) (*Planner, error) {
	return &Planner{
		Site:        site,
		Constraints: constraints,
	}, nil
}

// Observable returns true if all constraints are satisfied for obj at time t.
func (p *Planner) Observable(obj Observable, t time.Time) (bool, error) {
	eval, err := IsObservable(obj, t, p.Site, p.Constraints...)
	if err != nil {
		return false, err
	}
	return eval.Observable, nil
}

// FilterObservable returns the subset of objects that satisfy all constraints
// at the given time.
func (p *Planner) FilterObservable(objects []Observable, t time.Time) ([]Observable, error) {
	var filtered []Observable
	for _, obj := range objects {
		ok, err := p.Observable(obj, t)
		if err != nil {
			return nil, err
		}
		if ok {
			filtered = append(filtered, obj)
		}
	}
	return filtered, nil
}

// RankedObject pairs an object with its observability score.
type RankedObject struct {
	Object Observable
	Score  float64 // e.g., peak altitude in degrees
}

// RankObservable ranks objects by their maximum altitude within the given
// time window. Only objects that satisfy constraints at least once in the
// window are included.
func (p *Planner) RankObservable(objects []Observable, start, end time.Time) ([]RankedObject, error) {
	var ranked []RankedObject
	for _, obj := range objects {
		// TransitEstimate expects coord.Object for now.
		skyObj, ok := obj.(coord.Object)
		if !ok {
			return nil, fmt.Errorf("object %T does not implement coord.Object required for ranking", obj)
		}

		transitTime, peakAlt, err := TransitEstimate(skyObj, p.Site, start, end)
		if err != nil {
			return nil, err
		}

		ok, err = p.Observable(obj, transitTime)
		if err != nil {
			return nil, err
		}

		if ok {
			ranked = append(ranked, RankedObject{
				Object: obj,
				Score:  peakAlt.Degrees(),
			})
		}
	}

	// Sort by score descending
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].Score > ranked[j].Score
	})

	return ranked, nil
}

// Evaluation represents the aggregated result of multiple constraint checks.
type Evaluation struct {
	// Observable is true if all evaluated constraints passed.
	Observable bool
	// Results contains the individual results for each
	Results []Result
	// Position is the ICRS position of the object at evaluation time.
	Position *coord.ICRS
	// AltAz is the locally observed horizontal coordinates at evaluation time.
	AltAz *coord.AltAz
}

// IsObservable evaluates all provided constraints against a target at a specific
// time and site. It returns an Evaluation containing the outcome and individual
// constraint results.
//
// It only returns an error if a constraint check fails due to a technical error
// (e.g., ephemeris lookup failure), not if a constraint is simply not satisfied.
func IsObservable(
	obj Observable,
	t time.Time,
	site *Site,
	constraints ...Constraint,
) (Evaluation, error) {
	pos, err := obj.Position(t)
	if err != nil {
		return Evaluation{}, err
	}
	ctx := coord.NewContext(t, site.Location(), site.Atmosphere())
	altAz, err := ctx.ICRSToAltAz(pos)
	if err != nil {
		return Evaluation{}, err
	}

	eval := Evaluation{
		Observable: true,
		Results:    make([]Result, 0, len(constraints)),
		Position:   pos,
		AltAz:      altAz,
	}

	for _, c := range constraints {
		res, err := c.Check(obj, t, site)
		if err != nil {
			return Evaluation{}, err
		}
		eval.Results = append(eval.Results, res)
		if !res.Pass {
			eval.Observable = false
		}
	}

	return eval, nil
}

// ScoredTarget pairs an Observable with its calculated desirability score.
type ScoredTarget struct {
	Object Observable
	Score  float64
}

// Prioritized is an optional interface that targets can implement to provide
// a base priority for scoring.
type Prioritized interface {
	Priority() float64
}

// ScoreObservable calculates a desirability score for a target at a given time and site.
//
// Scoring methodology:
// 1. If the target is not observable (fails any constraint), score is 0.
// 2. Base score is the altitude in degrees (0 to 90).
// 3. If the target implements Prioritized, the score is multiplied by the priority.
// 4. A small bonus is added for Moon separation if evaluate-able (not implemented in v1 scoring core yet).
//
// This provides a transparent, altitude-first ranking that respects user-defined priorities.
func ScoreObservable(
	obj Observable,
	t time.Time,
	site *Site,
	constraints ...Constraint,
) (float64, error) {
	eval, err := IsObservable(obj, t, site, constraints...)
	if err != nil {
		return 0, err
	}

	if !eval.Observable {
		return 0, nil
	}

	score := eval.AltAz.Alt().Degrees()

	// Apply priority if available
	if p, ok := obj.(Prioritized); ok {
		score *= p.Priority()
	}

	return score, nil
}

// RankObservables evaluates and ranks a list of targets based on their observability
// score at a specific time and site.
func RankObservables(
	objs []Observable,
	t time.Time,
	site *Site,
	constraints ...Constraint,
) ([]ScoredTarget, error) {
	var scored []ScoredTarget
	for _, obj := range objs {
		s, err := ScoreObservable(obj, t, site, constraints...)
		if err != nil {
			return nil, err
		}
		if s > 0 {
			scored = append(scored, ScoredTarget{
				Object: obj,
				Score:  s,
			})
		}
	}

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	return scored, nil
}

// Window represents a contiguous time interval.
type Window struct {
	Start time.Time
	End   time.Time
}

// Duration returns the duration of the window as a standard time.Duration.
func (w Window) Duration() time.Duration {
	return w.End.Sub(w.Start)
}

// ObservableWindows computes the time intervals where the target satisfies all
// provided constraints by sampling the range [start, end] at the given cadence.
//
// grouping logic groups contiguous observable intervals into windows.
// For v1, this is a simple sampled search engine, not an exact event solver.
func ObservableWindows(
	obj Observable,
	start, end time.Time,
	step time.Duration,
	site *Site,
	constraints ...Constraint,
) ([]Window, error) {
	if step <= 0 {
		return nil, fmt.Errorf("step could not be negative")
	}

	var windows []Window
	inWindow := false
	var windowStart time.Time

	t := start
	for t.Before(end) || t.Equal(end) {
		eval, err := IsObservable(obj, t, site, constraints...)
		if err != nil {
			return nil, err
		}

		if eval.Observable && !inWindow {
			windowStart = t
			inWindow = true
		} else if !eval.Observable && inWindow {
			windows = append(windows, Window{
				Start: windowStart,
				End:   t,
			})
			inWindow = false
		}

		t = t.Add(step)
	}

	// Close the final window if the target was observable at the end of the range.
	if inWindow {
		windows = append(windows, Window{
			Start: windowStart,
			End:   end,
		})
	}

	return windows, nil
}
