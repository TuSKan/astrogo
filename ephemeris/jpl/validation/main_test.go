package jpl_test

import (
	"os"
	"testing"

	"github.com/TuSKan/astrogo/remote"
)

// TestMain grants download consent for this package's network/validation
// test suites, which construct real jpl.Provider instances against
// planetary and small-body kernels — a network/cache dependency that
// predates remote's consent-gating (see ephemeris/jpl's own TestMain for
// the same rationale). This file carries no build tag so it always
// compiles; granting consent here is harmless when no network/validation
// test actually runs (default go test ./... has nothing in this package
// to execute without those tags).
//
// Consent for IERSFinals2000A is granted here too: regression_test.go's
// TestScientificStability (validation-tagged) compares topocentric
// alt/az against a static Horizons corpus at sub-arcsecond tolerance,
// which needs real DUT1/polar motion, not the zero-EOP fallback. No
// explicit fetch call is needed — the first Time.EOP()/UTC()/UT1() query
// any test in this package makes now triggers time's automatic lazy load
// (disk cache, then this granted consent, then zero-EOP degradation),
// exactly as if a real coord.Context construction had asked for it
// directly. A lazy-load failure (offline, unreachable) only degrades to
// zero EOP with a one-time warning; the accuracy-sensitive tests still
// run and may fail on tolerance in that case, same as running without
// network access ever did.
func TestMain(m *testing.M) {
	remote.EnableDownloads(remote.NAIFSPK, 0)
	remote.EnableDownloads(remote.NAIFLSK, 0)
	remote.EnableDownloads(remote.JPLHorizons, 0)
	remote.EnableDownloads(remote.IERSFinals2000A, 0)

	os.Exit(m.Run())
}
