package testutil_test

import (
	"net"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// TestReachableFindsAListener is the ordinary path: something is there.
func TestReachableFindsAListener(t *testing.T) {
	t.Parallel()

	var lc net.ListenConfig

	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	t.Cleanup(func() { _ = ln.Close() })

	if !testutil.Reachable(ln.Addr().String()) {
		t.Errorf("Reachable(%s) = false for a listener this test is holding open", ln.Addr())
	}
}

// TestReachableReportsNothingThere exercises both dial attempts.
//
// A closed port fails over "tcp" and then over the IPv4 fallback, so this
// covers the second attempt as well as the answer. Port 1 on loopback is
// refused immediately rather than timing out, so the test costs nothing.
func TestReachableReportsNothingThere(t *testing.T) {
	t.Parallel()

	if testutil.Reachable("127.0.0.1:1") {
		t.Error("Reachable = true for a port with no listener")
	}
}

// TestReachableFallsBackToIPv4 is the defect this fallback exists for.
//
// Go's dual-stack dialer resolves AAAA and A together, and on a network where
// the AAAA lookup stalls the whole probe budget goes to a query for a record
// that does not exist. Measured on a developer machine:
// irsa.ipac.caltech.edu has one A record and no AAAA, "tcp" timed out after
// the full five seconds, and "tcp4" reached the very same address in 250 ms.
//
// The consequence was silence rather than slowness — every IRSA-dependent
// suite skipped on a service that was answering, and a skip reads as a pass.
//
// The stall itself cannot be reproduced without that resolver, so what is
// pinned here is the property that makes the fallback safe: an address only
// IPv4 can reach must still be found.
func TestReachableFallsBackToIPv4(t *testing.T) {
	t.Parallel()

	var lc net.ListenConfig

	ln, err := lc.Listen(t.Context(), "tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen tcp4: %v", err)
	}

	t.Cleanup(func() { _ = ln.Close() })

	if !testutil.Reachable(ln.Addr().String()) {
		t.Errorf("Reachable(%s) = false for an IPv4-only listener", ln.Addr())
	}
}
