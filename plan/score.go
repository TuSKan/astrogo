package plan

import (
	"sort"

	"github.com/TuSKan/astrogo/constraint"
	"github.com/TuSKan/astrogo/observatory"
	"github.com/TuSKan/astrogo/sky"
	"github.com/TuSKan/astrogo/target"
	"github.com/TuSKan/astrogo/time"
)

// ScoredTarget pairs an Observable with its calculated desirability score.
type ScoredTarget struct {
	Object target.Observable
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
	obj target.Observable,
	t time.Time,
	site observatory.Site,
	constraints ...constraint.Constraint,
) (float64, error) {
	eval, err := IsObservable(obj, t, site, constraints...)
	if err != nil {
		return 0, err
	}

	if !eval.Observable {
		return 0, nil
	}

	// Calculate altitude
	pos, err := obj.Position(t)
	if err != nil {
		return 0, err
	}
	altAz, err := sky.AltAz(pos, t, site)
	if err != nil {
		return 0, err
	}
	score := altAz.Alt.Degrees()

	// Apply priority if available
	if p, ok := obj.(Prioritized); ok {
		score *= p.Priority()
	}

	return score, nil
}

// RankObservables evaluates and ranks a list of targets based on their observability
// score at a specific time and site.
func RankObservables(
	objs []target.Observable,
	t time.Time,
	site observatory.Site,
	constraints ...constraint.Constraint,
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
