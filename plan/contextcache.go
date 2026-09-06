package plan

import (
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// ctxRefresh is how far a derived [coord.Context] may sit from the
// [coord.NewContext] it was derived from before that base is rebuilt.
//
// [coord.Context.AtTime] holds precession-nutation and aberration fixed, which
// costs ≲0.1″ per hour of separation from the base epoch — dominated by
// nutation's ~13.66-day term, with precession and the reused DUT1/polar-motion
// an order of magnitude smaller. One hour therefore bounds the error at ≲0.1″,
// which at the horizon's steepest crossing rate is under 0.01 s of rise/set
// bias: a hundredfold margin against the 1 s tolerance the solvers refine to,
// and against the minute-level USNO/NASA references this package is checked
// against.
//
// What it buys is the ~91 µs Apco13 rebuild, once an hour instead of once per
// sample and once per bisection iteration.
const ctxRefresh = 1 * time.Hour

// newContextCache returns a function handing out a [coord.Context] for any
// instant, rebuilding the expensive base only when t has drifted more than
// [ctxRefresh] from the last rebuild and deriving every other call with
// [coord.Context.AtTime].
//
// A closure over one base rather than a keyed cache: callers here sweep a
// window in order and then refine crossings inside it, so the useful hit rate
// comes from locality alone, and nothing has to decide when an entry expires.
// The drift test is absolute, so a solver stepping backwards inside a bracket
// is served the same way a forward sample is.
//
// One cache per atmosphere. A Context carries the refraction inputs it was
// built with, so deriving a refracted look angle from a geometric base would
// silently answer with the wrong atmosphere — which is why solveVisibility
// keeps separate caches for its geometric rise/set search and its refracted
// hour-angle search rather than sharing one.
func newContextCache(site *coord.Geodetic, atm atmosphere.Refraction) func(time.Time) *coord.Context {
	var base *coord.Context

	return func(t time.Time) *coord.Context {
		if base == nil || t.Sub(base.Time()).Abs() > ctxRefresh {
			base = coord.NewContext(t, site, atm)
		}

		return base.AtTime(t)
	}
}
