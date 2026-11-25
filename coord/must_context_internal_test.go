package coord

import (
	"testing"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/epoch"
)

// mustContext is NewContext for a test, failing it on the error
// NewContext returns.
func mustContext(tb testing.TB, at epoch.Time, site *Geodetic, atm atmosphere.Refraction) *Context {
	tb.Helper()

	ctx, err := NewContext(at, site, atm)
	if err != nil {
		tb.Fatal(err)
	}

	return ctx
}
