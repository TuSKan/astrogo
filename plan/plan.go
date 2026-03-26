package plan

import (
	"sort"
	stdtime "time" // Alias for standard library time to avoid conflict with astrogo/time

	"github.com/TuSKan/astrogo/constraint"
	"github.com/TuSKan/astrogo/observatory"
	"github.com/TuSKan/astrogo/sky"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/visibility"
)

// Observation pairs a Target with a specific observing time requirement.
type Observation struct {
	Target   sky.Target
	Duration stdtime.Duration
}

// Slot pairs a sky.Object with an observing Window.
type Slot struct {
	Object sky.Object
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
func (p *Planner) Observable(obj sky.Object, t time.Time) (bool, error) {
	return constraint.EvaluateAll(obj, t, p.Site, p.Constraints)
}

// FilterObservable returns the subset of objects that satisfy all constraints
// at the given time.
func (p *Planner) FilterObservable(objects []sky.Object, t time.Time) ([]sky.Object, error) {
	var filtered []sky.Object
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
	Object sky.Object
	Score  float64 // e.g., peak altitude in degrees
}

// RankObservable ranks objects by their maximum altitude within the given
// time window. Only objects that satisfy constraints at least once in the
// window are included.
func (p *Planner) RankObservable(objects []sky.Object, start, end time.Time) ([]RankedObject, error) {
	var ranked []RankedObject
	for _, obj := range objects {
		// TransitEstimate returns the time as well.
		transitTime, peakAlt, err := visibility.TransitEstimate(obj, p.Site, start, end)
		if err != nil {
			return nil, err
		}

		ok, err := p.Observable(obj, transitTime)
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
