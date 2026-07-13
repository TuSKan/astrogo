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
func TestMain(m *testing.M) {
	remote.EnableDownloads(remote.NAIFSPK, 0)
	remote.EnableDownloads(remote.NAIFLSK, 0)
	remote.EnableDownloads(remote.JPLHorizons, 0)

	os.Exit(m.Run())
}
