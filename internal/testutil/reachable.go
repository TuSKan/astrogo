package testutil

import (
	"net"
	"testing"
	"time"
)

// ReachableTimeout bounds the pre-check. It is deliberately short: the point
// is to find out whether a service is there at all, not to wait for a slow one.
const ReachableTimeout = 5 * time.Second

// RequireReachable skips the test unless host is accepting connections.
//
// Network-tagged tests in this repository never fail for someone else's
// downtime — an unreachable endpoint says nothing about the code under test.
// Every such test therefore opens a socket before doing anything else, and
// before this helper existed that pre-check was hand-written in a dozen files,
// each with its own copy of the same nolint directive.
//
// host is a "name:port" address. The connection is closed immediately; nothing
// is sent.
func RequireReachable(t *testing.T, host string) {
	t.Helper()

	//nolint:noctx // a liveness probe, not a request that should carry a deadline
	conn, err := net.DialTimeout("tcp", host, ReachableTimeout)
	if err != nil {
		t.Skipf("%s is unreachable: %v", host, err)
	}

	_ = conn.Close()
}
