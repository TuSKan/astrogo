package fink

import (
	"math"
	"testing"
)

func TestRecordToTarget_RBand(t *testing.T) {
	p := New()
	rec := &ssoRecord{
		Name:   "Benoitcarry",
		Number: 8467,
		H1:     15.0,
		H2:     14.5,
		G1_1:   0.20,
		G1_2:   0.25,
		G2_1:   0.40,
		G2_2:   0.35,
		R:      0.75,
		Alpha0: 30.0,
		Delta0: 45.0,
		AB:     1.2,
		AC:     1.5,
		Fit:    0,
		Status: 2,
		NObs:   327,
		RMS:    0.06,
	}

	tgt := p.recordToTarget(rec)

	if tgt.Name != "Benoitcarry" {
		t.Errorf("Name = %q, want Benoitcarry", tgt.Name)
	}

	if tgt.ID != "8467" {
		t.Errorf("ID = %q, want 8467", tgt.ID)
	}

	if tgt.Catalog != "fink" {
		t.Errorf("Catalog = %q, want fink", tgt.Catalog)
	}

	// Should prefer r-band (filter 2).
	if !tgt.HasH || tgt.H != 14.5 {
		t.Errorf("H = %.1f (HasH=%v), want 14.5", tgt.H, tgt.HasH)
	}

	if !tgt.HasG1G2 || tgt.G1 != 0.25 || tgt.G2 != 0.35 {
		t.Errorf("G1=%.2f G2=%.2f (HasG1G2=%v), want 0.25/0.35", tgt.G1, tgt.G2, tgt.HasG1G2)
	}

	if !tgt.HasSpin || tgt.SpinRA != 30.0 || tgt.SpinDec != 45.0 {
		t.Errorf("SpinRA=%.1f SpinDec=%.1f (HasSpin=%v), want 30/45", tgt.SpinRA, tgt.SpinDec, tgt.HasSpin)
	}

	if !tgt.HasOblateness || tgt.Oblateness != 0.75 {
		t.Errorf("Oblateness=%.2f (Has=%v), want 0.75", tgt.Oblateness, tgt.HasOblateness)
	}
}

func TestRecordToTarget_GBandFallback(t *testing.T) {
	p := New()
	rec := &ssoRecord{
		Number: 99,
		H1:     16.0,
		H2:     math.NaN(),
		G1_1:   0.30,
		G1_2:   math.NaN(),
		G2_1:   0.45,
		G2_2:   math.NaN(),
		R:      math.NaN(),
		Alpha0: math.NaN(),
		Delta0: math.NaN(),
	}

	tgt := p.recordToTarget(rec)

	// Should fall back to g-band.
	if !tgt.HasH || tgt.H != 16.0 {
		t.Errorf("H = %.1f, want 16.0 (g-band fallback)", tgt.H)
	}

	if !tgt.HasG1G2 || tgt.G1 != 0.30 {
		t.Errorf("G1 = %.2f, want 0.30 (g-band fallback)", tgt.G1)
	}

	if tgt.HasSpin {
		t.Error("HasSpin should be false for NaN spin values")
	}

	if tgt.HasOblateness {
		t.Error("HasOblateness should be false for NaN R")
	}
}

func TestLookup(t *testing.T) {
	p := New()
	p.mu.Lock()
	p.loaded = true
	p.byNumber = map[int64]*ssoRecord{
		8467: {Name: "Benoitcarry", Number: 8467, H2: 14.5, G1_2: 0.25, G2_2: 0.35, R: 0.75, Alpha0: 30, Delta0: 45},
		4:    {Name: "Vesta", Number: 4, H2: 3.2, G1_2: 0.30, G2_2: 0.38, R: 0.90, Alpha0: 300, Delta0: 60},
	}
	p.byName = map[string]*ssoRecord{
		"benoitcarry": p.byNumber[8467],
		"vesta":       p.byNumber[4],
	}
	p.mu.Unlock()

	// By number.
	rec := p.lookupCached("8467")
	if rec == nil || rec.Number != 8467 {
		t.Fatal("lookup by number failed")
	}

	// By name (case-insensitive).
	rec = p.lookupCached("Vesta")
	if rec == nil || rec.Number != 4 {
		t.Fatal("lookup by name failed")
	}

	// Not found.
	rec = p.lookupCached("nonexistent")
	if rec != nil {
		t.Fatal("lookup should return nil for unknown object")
	}
}

func TestProvider_Interface(t *testing.T) {
	p := New()
	if p.Name() != "fink" {
		t.Errorf("Name = %q, want fink", p.Name())
	}

	caps := p.Capabilities()
	if len(caps) != 2 {
		t.Errorf("Capabilities count = %d, want 2", len(caps))
	}
}
