package plan

import (
	"github.com/TuSKan/astrogo/coord"

	"github.com/TuSKan/astrogo/time"
)

// Environment bundles physical parameters for constraint evaluation.
// In v1 it's empty, but in v2 it will contain weather, moon position, etc.
type Environment struct {
	// Future: Moon coord.ICRS
	// Future: Sun  coord.ICRS
}

// EvalContext carries memoized data across multiple constraint evaluations
// for the same (object, time, site) triplet. It wraps a coord.Context
// to reuse the precomputed ASTROM parameters.
type EvalContext struct {
	Object coord.Object
	Time   time.Time
	Site   *Site
	Env    *Environment
	Ctx    *coord.Context

	// Memoized values
	icrs  *coord.ICRS
	altAz *coord.AltAz
	err   error
}

// NewEvalContext creates a bare context for evaluation.
func NewEvalContext(obj coord.Object, t time.Time, site *Site, env *Environment) *EvalContext {
	return &EvalContext{
		Object: obj,
		Time:   t,
		Site:   site,
		Env:    env,
		Ctx:    coord.NewContext(t, site.Location(), site.Atmosphere()),
	}
}

// NewEvalContextWith creates a context that reuses an existing coord.Context.
func NewEvalContextWith(obj coord.Object, t time.Time, site *Site, env *Environment, ctx *coord.Context) *EvalContext {
	return &EvalContext{
		Object: obj,
		Time:   t,
		Site:   site,
		Env:    env,
		Ctx:    ctx,
	}
}

// ICRS returns the (memoized) ICRS coordinates.
func (c *EvalContext) ICRS() (*coord.ICRS, error) {
	if c.icrs != nil {
		return c.icrs, c.err
	}
	icrs, err := c.Object.ICRS(c.Time)
	c.icrs = icrs
	c.err = err
	return icrs, err
}

// AltAz returns the (memoized) AltAz coordinates.
func (c *EvalContext) AltAz() (*coord.AltAz, error) {
	if c.altAz != nil {
		return c.altAz, c.err
	}
	icrs, err := c.ICRS()
	if err != nil {
		return nil, err
	}
	aa, err := c.Ctx.ICRSToAltAz(icrs)
	c.altAz = aa
	c.err = err
	return aa, err
}
