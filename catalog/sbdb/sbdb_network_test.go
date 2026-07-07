//go:build network
// +build network

package sbdb

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

// requireSBDB skips the test when the JPL SBDB API is unreachable — per
// this project's network test policy, a reachability failure must never
// fail CI outright.
func requireSBDB(t *testing.T) {
	t.Helper()

	conn, err := net.DialTimeout("tcp", "ssd-api.jpl.nasa.gov:443", 5*time.Second)
	if err != nil {
		t.Skipf("SBDB unreachable, skipping live test: %v", err)
	}

	_ = conn.Close()
}

func TestSBDBNetworkResolve(t *testing.T) {
	requireSBDB(t)

	prov := New()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := resolve.ObjectRequest{Query: "Aten", Limit: 1}
	iter := prov.ResolveObject(ctx, req)

	var targets []resolve.Target
	iter(func(tar resolve.Target, err error) bool {
		if err != nil {
			t.Fatalf("Live network failed: %v", err)
		}
		targets = append(targets, tar)
		return true
	})

	if len(targets) == 0 {
		t.Fatalf("Expected remote result for Halley")
	}

	tgt := targets[0]
	if tgt.ID == "" {
		t.Errorf("Expected ID populated from live server")
	}
}
