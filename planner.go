package astrogo

import (
	"time"

	"github.com/TuSKan/astrogo/coords"
)

// Planner aggregates native structural evaluations tracking continuous Location mapping automatically intersecting Filter boundaries accurately.
type Planner struct {
	Location    coords.Location
	Constraints []Constraint
}

// IsObservable resolves holistic structural queries returning boolean evaluations bounding targets directly natively.
// Evaluates strict short-circuit logics dropping evaluation flows cleanly mapping boolean interfaces intrinsically.
func (p *Planner) IsObservable(target Target, t time.Time) (bool, error) {
	for _, constraint := range p.Constraints {
		ok, err := constraint.Evaluate(target, t, p.Location)
		if err != nil {
			return false, err // Bubble pipeline math evaluation logic rejections transparently
		}
		if !ok {
			return false, nil // Immediate degradation upon filter rejection seamlessly
		}
	}
	return true, nil
}
