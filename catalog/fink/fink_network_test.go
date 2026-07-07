//go:build network

//go test -tags network ./catalog/fink

package fink

import (
	"net"
	"testing"
	"time"
)

// requireFink skips the test when the FINK SSOFT API is unreachable — per
// this project's network test policy, a reachability failure must never
// fail CI outright.
func requireFink(t *testing.T) {
	t.Helper()

	conn, err := net.DialTimeout("tcp", "api.ztf.fink-portal.org:443", 5*time.Second)
	if err != nil {
		t.Skipf("FINK unreachable, skipping live test: %v", err)
	}

	_ = conn.Close()
}

func TestFINKProvider_SingleObjectJSON(t *testing.T) {
	requireFink(t)

	p := New()

	// Fast single-object JSON query.
	tgt, ok := p.Resolve("8467")
	if !ok {
		t.Fatal("Resolve(8467) failed")
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
	tgt2, ok := p.Resolve("Benoitcarry")
	if !ok {
		t.Fatal("Resolve by name failed")
	}
	if tgt2.H != tgt.H {
		t.Errorf("Name/number lookup mismatch: H=%f vs %f", tgt2.H, tgt.H)
	}
}
