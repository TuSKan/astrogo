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

// TestSimbadNetworkSearch runs Search against the live service. Nothing did
// before #705, and every Search had failed with an HTTP 400 for five weeks:
// the query ordered by a qualified column, which SIMBAD's parser rejects and
// the offline tests, reading only the query's text, could not see. A 400 is
// not an upstream failure, so it fails here.
func TestSimbadNetworkSearch(t *testing.T) {
	requireSimbad(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	targets, err := New().Search(ctx, "HD 18973")
	testutil.SkipOnUpstreamFailure(t, err)

	if err != nil {
		t.Fatalf("live Search failed: %v", err)
	}

	for _, tgt := range targets {
		if tgt.Name == "HD 189733" {
			return
		}
	}

	t.Errorf("Search(%q) returned %d targets, none HD 189733, which SIMBAD lists for that prefix", "HD 18973", len(targets))
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

// Objects resolved live take their kind from SIMBAD's own hierarchy (#601).
// Each is one the old string match got wrong or flattened: Aldebaran is "LP?",
// a long-period variable candidate, and came back KindOther; M33 ("GiG") and
// M77 ("Sy2") are galaxies that came back KindOther; the Pleiades and M13
// were a generic cluster.
func TestSimbadKindsFromTheLiveHierarchy(t *testing.T) {
	requireSimbad(t)

	prov := New()

	cases := []struct {
		query string
		want  resolve.Kind
	}{
		{"Aldebaran", resolve.KindStar},
		{"M 33", resolve.KindGalaxy},
		{"M 77", resolve.KindGalaxy},
		{"M 45", resolve.KindOpenCluster},
		{"M 13", resolve.KindGlobularCluster},
		{"M 1", resolve.KindSupernovaRemnant},
		{"M 42", resolve.KindNebula},
		{"Sirius", resolve.KindStar},
	}

	for _, c := range cases {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		got, err := prov.Resolve(ctx, c.query)

		cancel()
		testutil.SkipOnUpstreamFailure(t, err)

		if err != nil {
			t.Errorf("Resolve(%q): %v", c.query, err)
			continue
		}

		if got.Kind != c.want {
			t.Errorf("Resolve(%q) = %s, kind %s; want %s", c.query, got.ID, got.Kind, c.want)
		}
	}
}

// A bright search with no limit returns every object brighter than the
// bound. It used to return 100, the brightest of them down to V 2.46,
// whatever the bound (#603). SIMBAD held 953 objects brighter than V 4.5 when
// this was written; the bound below leaves room for its catalog to change.
func TestSimbadSearchBrightReturnsEveryObject(t *testing.T) {
	requireSimbad(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var n int

	New().SearchBright(ctx, resolve.BrightRequest{MaxVMag: 4.5})(func(tgt resolve.Target, err error) bool {
		testutil.SkipOnUpstreamFailure(t, err)

		if err != nil {
			t.Fatalf("SearchBright: %v", err)
		}

		if !tgt.HasCoord {
			t.Errorf("%s has no position; the bright query should leave such objects out", tgt.ID)
		}

		n++

		return true
	})

	if n < 800 {
		t.Errorf("SearchBright(V < 4.5) returned %d objects; SIMBAD holds some 950", n)
	}

	t.Logf("SIMBAD objects brighter than V 4.5: %d", n)
}
