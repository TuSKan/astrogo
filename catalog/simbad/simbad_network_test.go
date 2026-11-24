//go:build network

package simbad

import (
	"context"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/internal/testutil"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/remote"
)

// requireSimbad skips the test when the SIMBAD TAP endpoint is unreachable
// (DNS failure, firewall, transient outage) — per this project's network
// test policy, a reachability failure must never fail CI outright.
func requireSimbad(t *testing.T) {
	t.Helper()

	testutil.RequireReachable(t, "simbad.cds.unistra.fr:80")
}

// simbadControl is a request a working SIMBAD cannot reject, for
// testutil.SkipOnDegradedService: one main_id from basic, a table and column
// SIMBAD has served for as long as it has had a TAP service.
//
// requireSimbad's reachability check is not enough on its own. On 2026-11-10
// SIMBAD's whole web application answered 404, its home page included, while
// the host still accepted connections, and these tests failed on the outage
// instead of skipping it (#1026). A 404 alone cannot say whose fault it is;
// the control can, since it fails too only when SIMBAD is the one failing.
//
// Its deadline starts when it runs, not when the test did.
func simbadControl(t *testing.T) func() error {
	t.Helper()

	syncURL, err := remote.URL(remote.SIMBAD)
	if err != nil {
		t.Fatalf("SIMBAD endpoint: %v", err)
	}

	// The mirrors remote asks while SIMBAD's own host is not serving, so the
	// control asks the host that answered the request under test.
	ep, _ := remote.Lookup(remote.SIMBAD)

	return func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		return testutil.TAPControl(ctx, syncURL, "basic", "main_id", ep.Mirrors...)()
	}
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
		testutil.SkipOnDegradedService(t, err, simbadControl(t))

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
	testutil.SkipOnDegradedService(t, err, simbadControl(t))

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
		testutil.SkipOnDegradedService(t, err, simbadControl(t))

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
		testutil.SkipOnDegradedService(t, err, simbadControl(t))

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
		testutil.SkipOnDegradedService(t, err, simbadControl(t))

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

// TestSimbadReportsItsGaiaStarsAtGaiasEpochLive holds SIMBAD's convention
// for a position it takes from Gaia to what readKinematics assumes: moved to
// J2000.0 without the radial velocity, with the proper motion at Gaia's
// epoch. Undone, Barnard's star and 61 Cyg A land on their Gaia DR3 rows at
// J2016.0 within Gaia's stated error. If SIMBAD changes the convention, or
// cites a Gaia release gaiaCoordinateEpochs does not know, this fails.
func TestSimbadReportsItsGaiaStarsAtGaiasEpochLive(t *testing.T) {
	requireSimbad(t)

	prov := New()

	for _, query := range []string{"Barnard's star", "61 Cyg A"} {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

		var got []resolve.Target

		prov.ResolveObject(ctx, resolve.ObjectRequest{Query: query, Limit: 1})(func(tg resolve.Target, err error) bool {
			testutil.SkipOnDegradedService(t, err, simbadControl(t))

			if err != nil {
				t.Fatalf("%s: %v", query, err)
			}

			got = append(got, tg)

			return true
		})

		cancel()

		if len(got) != 1 {
			t.Fatalf("%s: %d targets, want 1", query, len(got))
		}

		tg := got[0]

		row, ok := gaiaDR3[tg.ID]
		if !ok {
			t.Fatalf("%s resolved to %q, which has no Gaia DR3 row here", query, tg.ID)
		}

		if !tg.Epoch.Equal(gaiaEpoch) {
			t.Errorf("%s is reported at %v, want Gaia's J2016.0", tg.ID, tg.Epoch)
		}

		d := masApart(tg.Coord, coord.NewICRS(angle.Deg(row.ra), angle.Deg(row.dec)))
		if d > row.raErr+row.decErr {
			t.Errorf("%s lands %.4f mas from its Gaia DR3 row at J2016.0, over its stated %.3f mas",
				tg.ID, d, row.raErr+row.decErr)
		}

		t.Logf("%s: %.4f mas from its Gaia DR3 row at J2016.0", tg.ID, d)
	}
}
