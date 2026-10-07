//go:build network

// go test -tags network ./catalog/fink

package fink

import (
	"context"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// requireFink skips the test when the FINK SSOFT API is unreachable — per
// this project's network test policy, a reachability failure must never
// fail CI outright.
func requireFink(t *testing.T) {
	t.Helper()

	testutil.RequireReachable(t, "api.ztf.fink-portal.org:443")
}

func TestFINKProvider_SingleObjectJSON(t *testing.T) {
	requireFink(t)

	p := New()

	// Fast single-object JSON query.
	tgt, err := p.Resolve(context.Background(), "8467")

	testutil.SkipOnUpstreamFailure(t, err)

	if err != nil {
		t.Fatalf("Resolve(8467): %v", err)
	}

	t.Logf("8467 %s:", tgt.Name)
	t.Logf("  H       = %.4f", tgt.H)
	t.Logf("  G1      = %.4f", tgt.G1)
	t.Logf("  G2      = %.4f", tgt.G2)
	t.Logf("  SpinRA  = %.2f°", tgt.SpinRA)
	t.Logf("  SpinDec = %.2f°", tgt.SpinDec)
	t.Logf("  R       = %.4f", tgt.Oblateness)

	if !tgt.HasH {
		t.Error("HasH should be true")
	}

	if !tgt.HasG1G2 {
		t.Error("HasG1G2 should be true")
	}

	if !tgt.HasSpin {
		t.Error("HasSpin should be true")
	}

	if !tgt.HasOblateness {
		t.Error("HasOblateness should be true")
	}

	// Physical bounds.
	if tgt.H < 5 || tgt.H > 25 {
		t.Errorf("H = %.2f out of plausible range [5,25]", tgt.H)
	}

	if tgt.G1 < 0 || tgt.G1 > 1 {
		t.Errorf("G1 = %.4f out of [0,1]", tgt.G1)
	}

	if tgt.G2 < 0 || tgt.G2 > 1 {
		t.Errorf("G2 = %.4f out of [0,1]", tgt.G2)
	}

	if tgt.Oblateness <= 0 || tgt.Oblateness > 1 {
		t.Errorf("R = %.4f out of (0,1]", tgt.Oblateness)
	}

	if tgt.SpinRA < 0 || tgt.SpinRA >= 360 {
		t.Errorf("SpinRA = %.2f out of [0,360)", tgt.SpinRA)
	}

	if tgt.SpinDec < -90 || tgt.SpinDec > 90 {
		t.Errorf("SpinDec = %.2f out of [-90,90]", tgt.SpinDec)
	}

	// Cross-check by name.
	tgt2, err := p.Resolve(context.Background(), "Benoitcarry")

	testutil.SkipOnUpstreamFailure(t, err)

	if err != nil {
		t.Fatalf("Resolve(Benoitcarry): %v", err)
	}

	if tgt2.H != tgt.H {
		t.Errorf("Name/number lookup mismatch: H=%f vs %f", tgt2.H, tgt.H)
	}
}

// The real bulk table, read the way Resolve falls back to it.
//
// Nothing tested this against FINK's own file until #596, and the fixture
// that stood in for it had chosen its own column types: FINK stores sso_number
// as a string, which the reader read as 0 for every one of SSOFT 2025.04's
// 151,924 rows, so the number index held a single entry. The version is
// pinned, so the count is fixed; 148,922 rows pass the fit and status filter.
func TestFINKBulkTableIndexesEveryAsteroid(t *testing.T) {
	requireFink(t)

	p := New()

	err := p.ensureLoaded(context.Background())
	testutil.SkipOnUpstreamFailure(t, err)

	if err != nil {
		t.Fatalf("ensureLoaded: %v", err)
	}

	if got := p.Count(); got < 100000 {
		t.Errorf("Count() = %d; SSOFT %s indexes some 150,000 asteroids", got, p.version)
	}

	rec := p.lookupCached("8467")
	if rec == nil || rec.Name != "Benoitcarry" {
		t.Fatalf("lookupCached(8467) = %+v, want Benoitcarry", rec)
	}

	if byName := p.lookupCached("Benoitcarry"); byName == nil || byName.Number != 8467 {
		t.Errorf("lookupCached(Benoitcarry) = %+v, want number 8467", byName)
	}

	t.Logf("SSOFT %s: %d asteroids indexed", p.version, p.Count())
}
