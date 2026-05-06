//go:build network
// +build network

package simbad

import (
	"context"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

func TestSimbadNetworkResolve(t *testing.T) {
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
