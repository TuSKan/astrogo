package plan

import (
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// movingContext returns a function handing out a [coord.Context] for any
// instant: one Context, built on the first call and moved to each later
// instant with [coord.Context.SetTime].
//
// It used to be a cache that rebuilt its base an hour from the last rebuild
// and derived every other instant with AtTime, a new Context per call. The
// Context does both itself now, without allocating (#675), so what is left
// here is building it lazily, at the first instant a caller asks for: that
// keeps its epochs where the cache put them.
//
// Every call returns the same Context. A caller that keeps one across the
// next call sees it at the later instant; one that needs two instants at once
// copies it, c := *ctx.
//
// One per atmosphere. A Context carries the refraction inputs it was built
// with, so deriving a refracted look angle from a geometric Context would
// silently answer with the wrong atmosphere — which is why solveVisibility
// keeps separate ones for its geometric rise/set search and its refracted
// hour-angle search rather than sharing one.
func movingContext(site *coord.Geodetic, atm atmosphere.Refraction) func(time.Time) *coord.Context {
	var ctx *coord.Context

	return func(t time.Time) *coord.Context {
		if ctx == nil {
			ctx = coord.NewContext(t, site, atm)
		} else {
			ctx.SetTime(t)
		}

		return ctx
	}
}
