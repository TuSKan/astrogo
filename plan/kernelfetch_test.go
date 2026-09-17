//go:build integration

package plan_test

import (
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// requireKernel turns a provider-construction failure into the right kind of
// test outcome, and exists because getting that wrong cost this suite six red
// checks on a pull request that touched none of these packages (#348).
//
// # The service under test is not the service that fails
//
// Every test in this file's neighbours validates astrogo against an external
// reference — AstroPixels' phase tables, NASA's eclipse catalogues, USNO's
// almanac — and each already skips when *that* service cannot be reached.
// None of them guarded the step before: loading a DE44x kernel, which is a
// download from NAIF at jpl.nasa.gov and an entirely different service.
//
// So NAIF going down failed tests whose subject is a NASA eclipse page, with
// the message "Failed to create DE441 provider". Four of the six failures in
// #348 were that, and the repository's own rule is that external downtime must
// never read as a failure — CLAUDE.md, under the network tag: "never fail CI
// for external downtime".
//
// # Why skip rather than fall back
//
// plan/usno_test.go's newEph falls back to eph.Default() when a kernel will not
// load, which is right for a test that only needs *an* ephemeris. It is wrong
// here: these tests compare astrogo's DE441 answers against a published table
// to arcsecond or better, and the analytic default is not that. Silently
// swapping it in would turn an unreachable NAIF into a wrong-looking comparison
// against the right reference, which is worse than either a skip or a failure.
//
// # What stays fatal
//
// Anything that is not the network. A kernel that downloads and then fails to
// parse, a refused download, a missing body — all of those are astrogo's
// problem or the caller's, and all of them keep failing the build.
func requireKernel(t *testing.T, what string, err error) {
	t.Helper()

	if err == nil {
		return
	}

	if testutil.Unreachable(err) {
		t.Skipf("%s: NAIF is unreachable, so the kernel could not be fetched: %v "+
			"(external, not astrogo)", what, err)
	}

	t.Fatalf("%s: %v", what, err)
}
