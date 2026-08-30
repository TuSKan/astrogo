//go:build network

package simbad

import (
	"context"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/time"
)

// requireSimbad skips the test when the SIMBAD TAP endpoint is unreachable
// (DNS failure, firewall, transient outage) — per this project's network
// test policy, a reachability failure must never fail CI outright.
func requireSimbad(t *testing.T) {
	t.Helper()

	testutil.RequireReachable(t, "simbad.cds.unistra.fr:80")
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
		testutil.SkipOnUpstreamFailure(t, err)

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

// TestSimbadNetworkSearchBright is a live end-to-end check of
// BuildBrightQuery/ParseBrightCSV against the real TAP service — this is
// exactly the path a prior version got wrong twice (an ORDER BY the live
// parser rejects, then a VMag column-name case mismatch) with no live test
// to catch either, since the offline mock fixture matched the (wrong)
// assumption rather than the real API.
func TestSimbadNetworkSearchBright(t *testing.T) {
	requireSimbad(t)

	prov := New()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	iter := prov.SearchBright(ctx, resolve.BrightRequest{MaxVMag: 2, Limit: 20})

	var targets []resolve.Target

	iter(func(tgt resolve.Target, err error) bool {
		testutil.SkipOnUpstreamFailure(t, err)

		if err != nil {
			t.Fatalf("live SearchBright failed: %v", err)
		}

		targets = append(targets, tgt)

		return true
	})

	if len(targets) == 0 {
		t.Fatal("expected at least one real star brighter than mag 2 (e.g. Sirius, Canopus)")
	}

	for _, tgt := range targets {
		if !tgt.HasVMag {
			t.Errorf("target %q missing VMag from a live response", tgt.Name)
		}

		if tgt.VMag >= 2 {
			t.Errorf("target %q VMag = %v, want < 2 (magLimit)", tgt.Name, tgt.VMag)
		}

		if !tgt.HasCoord {
			t.Errorf("target %q missing Coord from a live response", tgt.Name)
		}
	}

	// Brightest-first ordering (the ORDER BY vmag ASC clause).
	for i := 1; i < len(targets); i++ {
		if targets[i-1].VMag > targets[i].VMag {
			t.Errorf("targets not sorted brightest-first at index %d: %v > %v", i, targets[i-1].VMag, targets[i].VMag)
		}
	}
}
