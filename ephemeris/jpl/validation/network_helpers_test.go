//go:build network

package jpl_test

import (
	"net"
	"testing"
	"time"
)

// requireHorizons skips the test when the JPL Horizons API is unreachable —
// per this project's network test policy, a reachability failure must
// never fail CI outright. Shared by every network-tagged test in this
// package, all of which compare against the same live Horizons endpoint.
func requireHorizons(t *testing.T) {
	t.Helper()

	conn, err := net.DialTimeout("tcp", "ssd.jpl.nasa.gov:443", 5*time.Second)
	if err != nil {
		t.Skipf("JPL Horizons unreachable, skipping live test: %v", err)
	}

	_ = conn.Close()
}
