//go:build network
// +build network

package jpl

import (
	"context"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/catalog/provider"
)

func TestJPLNetworkResolve(t *testing.T) {
	prov := New()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req := provider.ObjectRequest{Query: "Mars", Limit: 1}
	iter := prov.ResolveObject(ctx, req)

	var targets []provider.Target
	iter(func(tar provider.Target, err error) bool {
		if err != nil {
			t.Fatalf("Live network failed: %v", err)
		}
		targets = append(targets, tar)
		return true
	})

	if len(targets) == 0 {
		t.Fatalf("Expected remote result for Mars from JPL Horizons")
	}

	tgt := targets[0]
	if tgt.ID == "" {
		t.Errorf("Expected ID populated from live server")
	}
}
