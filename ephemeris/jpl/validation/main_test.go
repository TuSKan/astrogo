package jpl_test

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
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
// It also best-effort-fetches real IERS EOP data: regression_test.go's
// TestScientificStability (validation-tagged) compares topocentric
// alt/az against a static Horizons corpus at sub-arcsecond tolerance,
// which needs real DUT1/polar motion, not the zero-EOP fallback — no
// package here ever registers a Model itself. A fetch failure (offline,
// unreachable) only logs; the accuracy-sensitive tests still run against
// whatever's registered (ZeroModel by default) and may fail on tolerance
// in that case, same as running without network access ever did.
func TestMain(m *testing.M) {
	remote.EnableDownloads(remote.NAIFSPK, 0)
	remote.EnableDownloads(remote.NAIFLSK, 0)
	remote.EnableDownloads(remote.JPLHorizons, 0)
	remote.EnableDownloads(remote.IERSFinals2000A, 0)

	if err := time.Fetch(context.Background()); err != nil {
		log.Printf("ephemeris/jpl/validation: TestMain: best-effort IERS EOP fetch failed (%v); accuracy-sensitive tests may fail on tolerance", err)
	}

	os.Exit(m.Run())
}
