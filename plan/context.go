package plan

import (
	"github.com/TuSKan/astrogo/coord"

	"github.com/TuSKan/astrogo/time"
)

// EvalContext carries memoized data across multiple constraint evaluations
// for the same (object, time, site) triplet. It wraps a coord.Context
// to reuse the precomputed ASTROM parameters.
//
// Each memoized result (ICRS, AltAz) stores its own error to prevent
// one path's failure from contaminating another's return value.
type EvalContext struct {
	Time    time.Time
	Object  coord.Object
	icrsErr error
	altErr  error
	Site    *Site
	Ctx     *coord.Context
	icrs    coord.ICRS
	altAz   coord.AltAz
	icrsOk  bool
	altOk   bool
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
func (c *EvalContext) ICRS() (coord.ICRS, error) {
	if c.icrsOk || c.icrsErr != nil {
		return c.icrs, c.icrsErr
	}
	icrs, err := c.Object.ICRS(c.Time)
	c.icrs = icrs
	c.icrsOk = err == nil
	c.icrsErr = err
	return icrs, err
}

// AltAz returns the (memoized) AltAz coordinates.
func (c *EvalContext) AltAz() (coord.AltAz, error) {
	if c.altOk || c.altErr != nil {
		return c.altAz, c.altErr
	}
	icrs, err := c.ICRS()
	if err != nil {
		c.altErr = err
		return coord.AltAz{}, err
	}
	aa, err := c.Ctx.ICRSToAltAz(icrs)
	c.altAz = aa
	c.altOk = err == nil
	c.altErr = err
	return aa, err
}
