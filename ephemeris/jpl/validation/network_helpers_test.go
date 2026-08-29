//go:build network

package jpl_test

import (
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// horizonsHost is the endpoint every network-tagged test in this package
// probes before doing anything else. Named rather than repeated so a suite
// that has to record a NOT VERIFIED result before skipping — see
// TestObserverPrecisionMatrix — probes exactly the same address as the tests
// that simply skip.
const horizonsHost = "ssd.jpl.nasa.gov:443"

// requireHorizons skips the test when the JPL Horizons API is unreachable —
// per this project's network test policy, a reachability failure must
// never fail CI outright. Shared by every network-tagged test in this
// package, all of which compare against the same live Horizons endpoint.
func requireHorizons(t *testing.T) {
	t.Helper()

	testutil.RequireReachable(t, horizonsHost)
}
