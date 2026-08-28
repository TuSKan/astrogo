//go:build network

package jpl_test

import (
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// requireHorizons skips the test when the JPL Horizons API is unreachable —
// per this project's network test policy, a reachability failure must
// never fail CI outright. Shared by every network-tagged test in this
// package, all of which compare against the same live Horizons endpoint.
func requireHorizons(t *testing.T) {
	t.Helper()

	testutil.RequireReachable(t, "ssd.jpl.nasa.gov:443")
}
