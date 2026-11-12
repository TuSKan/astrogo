package fink

import (
	"math"
	"testing"
)

// An infinite value in a FINK record is no value, as NaN already was there
// (#1080).
//
// FINK reads an absent field as NaN and tested for NaN alone, but a string
// field of "Infinity" and a Parquet float can both carry an infinity, which
// passed and became an H of +Inf. Each band falls back to the other as it does
// for NaN, so a body with one usable band keeps it.
func TestRecordToTargetReadsAnInfinityAsAbsent(t *testing.T) {
	t.Parallel()

	nan, inf := math.NaN(), math.Inf(1)

	p := &Provider{}

	tgt := p.recordToTarget(&ssoRecord{
		Name: "probe", Number: 1,
		H2: inf, H1: 15.2,
		G1_2: inf, G2_2: 0.3, G1_1: 0.6, G2_1: 0.2,
		Alpha0: math.Inf(-1), Delta0: 40,
		R: inf,
	})

	if !tgt.HasH || tgt.H != 15.2 {
		t.Errorf("H = %v (HasH %t); an infinite r-band H must fall back to the g band's 15.2", tgt.H, tgt.HasH)
	}

	if !tgt.HasG1G2 || tgt.G1 != 0.6 || tgt.G2 != 0.2 {
		t.Errorf("G1, G2 = %v, %v (HasG1G2 %t); want the g band's 0.6, 0.2", tgt.G1, tgt.G2, tgt.HasG1G2)
	}

	if tgt.HasSpin {
		t.Errorf("an infinite spin-axis RA was kept: %v, %v", tgt.SpinRA, tgt.SpinDec)
	}

	if tgt.HasOblateness {
		t.Errorf("an infinite oblateness was kept: %v", tgt.Oblateness)
	}

	// Both bands unusable: no H at all, rather than an infinite one.
	none := p.recordToTarget(&ssoRecord{Name: "none", Number: 2, H2: inf, H1: nan})
	if none.HasH {
		t.Errorf("H = %v with HasH true, from two unusable bands", none.H)
	}
}
