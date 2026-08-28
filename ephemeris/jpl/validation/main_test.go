package jpl_test

import (
	"os"
	"testing"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/remote"
)

// kmPerAU converts the AU-valued state differences these suites measure into
// the kilometres that the reference routines' own accuracy figures are quoted
// in. The idiom is the one the constants package's doc demonstrates; Value is
// in metres.
//
// It lives in this file, which carries no build tag, because the validation-
// and network-tagged suites both need it and cannot see each other's files.
//
//nolint:gochecknoglobals // a derived constant, same convention as the rest of this repo's reference data
var kmPerAU = constants.IAU.AstronomicalUnit.Value / 1e3

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
	remote.EnableDownloads(0, remote.NAIFSPK)
	remote.EnableDownloads(0, remote.NAIFLSK)
	remote.EnableDownloads(0, remote.JPLHorizonsSPK)
	remote.EnableDownloads(0, remote.IERSFinals2000A)

	os.Exit(m.Run())
}
