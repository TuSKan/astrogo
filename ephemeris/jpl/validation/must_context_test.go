package validation_test

import (
	"testing"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// mustContext is coord.NewContext for a test, failing it on the error
// coord.NewContext returns.
func mustContext(tb testing.TB, at time.Time, site *coord.Geodetic, atm atmosphere.Refraction) *coord.Context {
	tb.Helper()

	ctx, err := coord.NewContext(at, site, atm)
	if err != nil {
		tb.Fatal(err)
	}

	return ctx
}
