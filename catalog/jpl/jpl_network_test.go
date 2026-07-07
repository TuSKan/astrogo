//go:build network
// +build network

package jpl

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

// requireHorizons skips the test when the JPL Horizons API is unreachable —
// per this project's network test policy, a reachability failure must
// never fail CI outright.
func requireHorizons(t *testing.T) {
	t.Helper()

	conn, err := net.DialTimeout("tcp", "ssd.jpl.nasa.gov:443", 5*time.Second)
	if err != nil {
		t.Skipf("JPL Horizons unreachable, skipping live test: %v", err)
	}

	_ = conn.Close()
}

// TestJPLNetworkResolve confirms the provider reaches the live Horizons API
// and — since result-text parsing isn't implemented (see ErrNotImplemented)
// — surfaces that explicitly rather than fabricating a Target. This is a
// live-service reachability check, not a "resolution works" check.
func TestJPLNetworkResolve(t *testing.T) {
	requireHorizons(t)

	prov := New()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := resolve.ObjectRequest{Query: "Mars", Limit: 1}
	iter := prov.ResolveObject(ctx, req)

	var gotErr error
	iter(func(_ resolve.Target, err error) bool {
		gotErr = err
		return false
	})

	if !errors.Is(gotErr, ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented from a reachable, decodable live response, got: %v", gotErr)
	}
}
