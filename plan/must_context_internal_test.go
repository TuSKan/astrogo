package plan

import (
	"testing"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/epoch"
)

// mustContext is coord.NewContext for a test, failing it on the error
// coord.NewContext returns.
func mustContext(tb testing.TB, at epoch.Time, site *coord.Geodetic, atm atmosphere.Refraction) *coord.Context {
	tb.Helper()

	ctx, err := coord.NewContext(at, site, atm)
	if err != nil {
		tb.Fatal(err)
	}

	return ctx
}

// mustMoving is movingContext for a test, from start, failing it on the
// error movingContext returns.
func mustMoving(tb testing.TB, start epoch.Time, site *coord.Geodetic, atm atmosphere.Refraction) func(epoch.Time) *coord.Context {
	tb.Helper()

	ctxAt, err := movingContext(start, site, atm)
	if err != nil {
		tb.Fatal(err)
	}

	return ctxAt
}
