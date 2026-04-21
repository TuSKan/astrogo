package plan

import (
	"github.com/TuSKan/astrogo/coord"

	"github.com/TuSKan/astrogo/time"
)

// EvalContext carries memoized data across multiple constraint evaluations
// for the same (object, time, site) triplet. It wraps a coord.Context
// to reuse the precomputed ASTROM parameters.
//
// Future extensions (weather conditions, Moon/Sun pre-computed positions)
// can be added as fields here without changing the public constructor
// signature — use functional options or builder methods when that time comes.
type EvalContext struct {
	Object coord.Object
	Time   time.Time
	Site   *Site
	Ctx    *coord.Context

	// Memoized values
	icrs  *coord.ICRS
	altAz *coord.AltAz
	err   error
}

// NewEvalContext creates a bare context for evaluation.
func NewEvalContext(obj coord.Object, t time.Time, site *Site) *EvalContext {
	return &EvalContext{
		Object: obj,
		Time:   t,
		Site:   site,
		Ctx:    coord.NewContext(t, site.Location(), site.Atmosphere()),
	}
}

// NewEvalContextWith creates a context that reuses an existing coord.Context.
func NewEvalContextWith(obj coord.Object, t time.Time, site *Site, ctx *coord.Context) *EvalContext {
	return &EvalContext{
		Object: obj,
		Time:   t,
		Site:   site,
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
