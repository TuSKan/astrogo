//go:build network
// +build network

package mast

import (
	"context"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/catalog"
)

func TestMastNetworkResolve(t *testing.T) {
	prov := New()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// STScI MAST Name Lookup for Vega
	req := catalog.ObjectRequest{Query: "Vega", Limit: 1}

	iter := prov.ResolveObject(ctx, req)
	var targets []catalog.Target
	iter(func(tar catalog.Target, err error) bool {
		if err != nil {
			t.Fatalf("Live network failed: %v", err)
		}
		targets = append(targets, tar)
		return true
	})

	if len(targets) == 0 {
		t.Fatalf("Expected resolving Vega through MAST CAOM")
	}

	if targets[0].Coord == nil {
		t.Fatalf("Expected ICRS coordinates mapped properly")
	}
}
