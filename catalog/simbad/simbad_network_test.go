//go:build network
// +build network

package simbad

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

// requireSimbad skips the test when the SIMBAD TAP endpoint is unreachable
// (DNS failure, firewall, transient outage) — per this project's network
// test policy, a reachability failure must never fail CI outright.
func requireSimbad(t *testing.T) {
	t.Helper()

	conn, err := net.DialTimeout("tcp", "simbad.cds.unistra.fr:80", 5*time.Second)
	if err != nil {
		t.Skipf("SIMBAD unreachable, skipping live test: %v", err)
	}

	_ = conn.Close()
}

func TestSimbadNetworkResolve(t *testing.T) {
	requireSimbad(t)

	prov := New()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Live network test requesting M31 over real internet TAP
	req := resolve.ObjectRequest{Query: "m31", Limit: 1}
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
		t.Fatalf("Expected at least 1 remote result for M31")
	}

	tgt := targets[0]
	if tgt.ID == "" {
		t.Errorf("Expected ID populated from live server")
	}
	if !tgt.HasCoord {
		t.Fatalf("Expected live coordinates for M31")
	}
}
