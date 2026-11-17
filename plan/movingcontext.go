package plan

import (
	"fmt"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// siteContext is the Context a constraint's Check evaluates in: site's, at t.
// A nil site is [ErrNilSite], and what [coord.NewContext] refuses is its
// error.
func siteContext(t time.Time, site *Site) (*coord.Context, error) {
	if site == nil {
		return nil, ErrNilSite
	}

	ctx, err := coord.NewContext(t, site.Location(), site.Refraction())
	if err != nil {
		return nil, fmt.Errorf("plan: context: %w", err)
	}

	return ctx, nil
}

// siteContexts is movingContext for site, from start, after the checks every
// search over [start, end] makes first: a non-finite or reversed interval is
// the error checkInterval gives, as it always was, and a nil site is
// [ErrNilSite].
func siteContexts(start, end time.Time, site *Site) (func(time.Time) *coord.Context, error) {
	if err := checkInterval(start, end); err != nil {
		return nil, err
	}

	if site == nil {
		return nil, ErrNilSite
	}

	return movingContext(start, site.Location(), site.Refraction())
}

// movingContext returns a function handing out a [coord.Context] for any
// instant: one Context, built at start and moved to each instant asked for
// with [coord.Context.SetTime]. What [coord.NewContext] refuses, a nil site, a
// time that is not finite or air that is not air, it refuses here, before any
// instant is asked for.
//
// It used to be a cache that rebuilt its base an hour from the last rebuild
// and derived every other instant with AtTime, a new Context per call. The
// Context does both itself now, without allocating (#675). It then built the
// Context at the first instant a caller asked for, which could not return
// NewContext's error (#1186); every caller asks for start, or for an instant
// after it, first.
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
func movingContext(start time.Time, site *coord.Geodetic, atm atmosphere.Refraction) (func(time.Time) *coord.Context, error) {
	ctx, err := coord.NewContext(start, site, atm)
	if err != nil {
		return nil, fmt.Errorf("plan: context: %w", err)
	}

	return func(t time.Time) *coord.Context {
		ctx.SetTime(t)

		return ctx
	}, nil
}
