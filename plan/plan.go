package plan

import (
	"sort"

	"github.com/TuSKan/astrogo/constraint"
	"github.com/TuSKan/astrogo/observatory"
	"github.com/TuSKan/astrogo/sky"
	"github.com/TuSKan/astrogo/target"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/visibility"
)

// Observation pairs a Target with a specific observing time requirement.
type Observation struct {
	Target   target.Observable
	Duration time.Duration
}

// Slot pairs a sky.Object with an observing Window.
type Slot struct {
	Object target.Observable
	Window visibility.Window
}

// Planner evaluates sky.Objects against a set of Constraints at a given Site.
type Planner struct {
	Site        observatory.Site
	Constraints []constraint.Constraint
}

// NewPlanner creates a new Planner for the given site and constraints.
func NewPlanner(site observatory.Site, constraints []constraint.Constraint) (*Planner, error) {
	return &Planner{
		Site:        site,
		Constraints: constraints,
	}, nil
}

// Observable returns true if all constraints are satisfied for obj at time t.
func (p *Planner) Observable(obj target.Observable, t time.Time) (bool, error) {
	eval, err := IsObservable(obj, t, p.Site, p.Constraints...)
	if err != nil {
		return false, err
	}
	return eval.Observable, nil
}

// FilterObservable returns the subset of objects that satisfy all constraints
// at the given time.
func (p *Planner) FilterObservable(objects []target.Observable, t time.Time) ([]target.Observable, error) {
	var filtered []target.Observable
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
	Object target.Observable
	Score  float64 // e.g., peak altitude in degrees
}

// RankObservable ranks objects by their maximum altitude within the given
// time window. Only objects that satisfy constraints at least once in the
// window are included.
func (p *Planner) RankObservable(objects []target.Observable, start, end time.Time) ([]RankedObject, error) {
	var ranked []RankedObject
	for _, obj := range objects {
		// TransitEstimate expects sky.Object for now.
		skyObj, ok := obj.(sky.Object)
		if !ok {
			continue // Skip objects that don't satisfy sky.Object
		}

		transitTime, peakAlt, err := visibility.TransitEstimate(skyObj, p.Site, start, end)
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
	// Results contains the individual results for each constraint.
	Results []constraint.Result
}

// IsObservable evaluates all provided constraints against a target at a specific
// time and site. It returns an Evaluation containing the outcome and individual
// constraint results.
//
// It only returns an error if a constraint check fails due to a technical error
// (e.g., ephemeris lookup failure), not if a constraint is simply not satisfied.
func IsObservable(
	obj target.Observable,
	t time.Time,
	site observatory.Site,
	constraints ...constraint.Constraint,
) (Evaluation, error) {
	eval := Evaluation{
		Observable: true,
		Results:    make([]constraint.Result, 0, len(constraints)),
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

// Window represents a contiguous time interval.
type Window struct {
	Start time.Time
	End   time.Time
}

// ObservableWindows computes the time intervals where the target satisfies all
// provided constraints by sampling the range [start, end] at the given cadence.
//
// grouping logic groups contiguous observable intervals into windows.
// For v1, this is a simple sampled search engine, not an exact event solver.
func ObservableWindows(
	obj target.Observable,
	start, end time.Time,
	step time.Duration,
	site observatory.Site,
	constraints ...constraint.Constraint,
) ([]Window, error) {
	if step <= 0 {
		return nil, nil // Or return an error if preferred, but let's be safe.
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
