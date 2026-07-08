//go:build network
// +build network

package vizier

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
)

// requireVizier skips the test when the VizieR TAP endpoint is unreachable —
// per this project's network test policy, a reachability failure must
// never fail CI outright.
func requireVizier(t *testing.T) {
	t.Helper()

	conn, err := net.DialTimeout("tcp", "tapvizier.u-strasbg.fr:80", 5*time.Second)
	if err != nil {
		t.Skipf("VizieR unreachable, skipping live test: %v", err)
	}

	_ = conn.Close()
}

func TestVizierNetworkConeSearch(t *testing.T) {
	requireVizier(t)

	prov := New()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Generic ConeSearch around M31 core
	req := resolve.ConeRequest{
		Center: coord.NewICRS(angle.Deg(10.684), angle.Deg(41.269)),
		Radius: angle.Deg(0.01), // Very tight 36 arcseconds
		Limit:  10,
	}

	iter := prov.ConeSearch(ctx, req)
	var count int
	iter(func(tar resolve.Target, err error) bool {
		if err != nil {
			t.Fatalf("Live network failed: %v", err)
		}
		count++
		return true
	})

	// VizieR 2MASS should return sources inside a 36-arcsecond radius of
	// Andromeda's core; parseCSV now really parses the response (see R22 fix).
	if count == 0 {
		t.Error("expected at least one 2MASS source within 36 arcseconds of Andromeda's core")
	}

	if count > 10 {
		t.Fatalf("Expected limit to be respected")
	}
}

// TestVizierNetworkConeSearch_RegisteredTable confirms ConeSearch works
// live against a second registered table (Hipparcos), not just the default
// 2MASS one.
func TestVizierNetworkConeSearch_RegisteredTable(t *testing.T) {
	requireVizier(t)

	prov := New()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// A 2-degree cone around M31's core comfortably contains a Hipparcos star.
	req := resolve.ConeRequest{
		Table:  "I/239/hip_main",
		Center: coord.NewICRS(angle.Deg(10.684), angle.Deg(41.269)),
		Radius: angle.Deg(2),
		Limit:  5,
	}

	iter := prov.ConeSearch(ctx, req)

	var count int

	iter(func(_ resolve.Target, err error) bool {
		if err != nil {
			t.Fatalf("live network failed: %v", err)
		}

		count++

		return true
	})

	if count == 0 {
		t.Error("expected at least one Hipparcos star within 2 degrees of Andromeda's core")
	}
}
