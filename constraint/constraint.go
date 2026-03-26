package constraint

import (
	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/observatory"
	"github.com/TuSKan/astrogo/sky"
	"github.com/TuSKan/astrogo/time"
)


// Constraint defines a requirement for an observation to be considered valid.
type Constraint interface {
	// Evaluate returns true if the constraint is satisfied for the target
	// at the given time and site. It uses the provided Context for
	// access to memoized coordinates.
	Evaluate(ctx *Context) (bool, error)
}

// MinAltitudeConstraint ensures an object is above a minimum altitude.
type MinAltitudeConstraint struct {
	MinAlt angle.Angle
}

func (c MinAltitudeConstraint) Evaluate(ctx *Context) (bool, error) {
	aa, err := ctx.AltAz()
	if err != nil {
		return false, err
	}
	return aa.Alt.Degrees() >= c.MinAlt.Degrees(), nil
}

// MaxAirmassConstraint ensures an object's airmass is below a maximum limit.
type MaxAirmassConstraint struct {
	MaxAirmass float64
}

func (c MaxAirmassConstraint) Evaluate(ctx *Context) (bool, error) {
	aa, err := ctx.AltAz()
	if err != nil {
		return false, err
	}
	am, err := sky.Airmass(aa.Alt)
	if err != nil {
		// If object is below horizon, airmass returns error, which correctly
		// means constraint is not satisfied.
		if err == sky.ErrBelowHorizon {
			return false, nil
		}
		return false, err
	}
	return am <= c.MaxAirmass, nil
}

// EvaluateAll checks if an object satisfies all provided constraints.
func EvaluateAll(obj sky.Object, t time.Time, site observatory.Site, constraints []Constraint) (bool, error) {
	ctx := NewContext(obj, t, site, nil)
	for _, c := range constraints {
		ok, err := c.Evaluate(ctx)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}
