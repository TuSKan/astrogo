package constraint

import (
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/observatory"
	"github.com/TuSKan/astrogo/sky"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/transform"
)

// Environment bundles physical parameters for constraint evaluation.
// In v1 it's empty, but in v2 it will contain weather, moon position, etc.
type Environment struct {
	// Future: Moon coord.ICRS
	// Future: Sun  coord.ICRS
}

// Context carries memoized data across multiple constraint evaluations
// for the same (object, time, site) triplet.
type Context struct {
	Object sky.Object
	Time   time.Time
	Site   observatory.Site
	Env    *Environment

	// Memoized values
	icrs  *coord.ICRS
	altAz *coord.AltAz
	err   error
}

// NewContext creates a bare context for evaluation.
func NewContext(obj sky.Object, t time.Time, site observatory.Site, env *Environment) *Context {
	return &Context{
		Object: obj,
		Time:   t,
		Site:   site,
		Env:    env,
	}
}

// ICRS returns the (memoized) ICRS coordinates.
func (c *Context) ICRS() (coord.ICRS, error) {
	if c.icrs != nil {
		return *c.icrs, c.err
	}
	icrs, err := c.Object.ICRS(c.Time)
	c.icrs = &icrs
	c.err = err
	return icrs, err
}

// AltAz returns the (memoized) AltAz coordinates.
func (c *Context) AltAz() (coord.AltAz, error) {
	if c.altAz != nil {
		return *c.altAz, c.err
	}
	icrs, err := c.ICRS()
	if err != nil {
		return coord.AltAz{}, err
	}
	aa, err := transform.ICRSToAltAz(icrs, c.Time, c.Site.Location())
	c.altAz = &aa
	c.err = err
	return aa, err
}
