package spk_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestSPKReader(t *testing.T) {
	// Bootstrap the download process robustly via the provider logic
	prov, err := jpl.NewProvider(core.Planets, "de440s")
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	t.Cleanup(func() {
		err := prov.Close()
		if err != nil {
			t.Errorf("failed to close provider: %v", err)
		}
	})

	spkPath := filepath.Join(prov.DataDir, "planets", "de440s.bsp")

	f, err := os.Open(spkPath)
	testutil.AssertNoError(t, err)

	t.Cleanup(func() {
		err := f.Close()
		if err != nil {
			t.Errorf("failed to close file: %v", err)
		}
	})

	r, err := spk.NewReader(f)
	testutil.AssertNoError(t, err)

	t.Cleanup(func() {
		err := r.Close()
		if err != nil {
			t.Errorf("failed to close reader: %v", err)
		}
	})

	if r.FileRec.ND != 2 || r.FileRec.NI != 6 {
		t.Errorf("expected ND=2, NI=6 for SPK, got ND=%d, NI=%d", r.FileRec.ND, r.FileRec.NI)
	}

	summaries, err := r.ReadSummaries()
	testutil.AssertNoError(t, err)

	if len(summaries) == 0 {
		t.Fatal("expected more than 0 summaries")
	}

	var segments []spk.Segment
	for _, sum := range summaries {
		segments = append(segments, spk.Segment{
			Target:    sum.Integers[0],
			Center:    sum.Integers[1],
			Frame:     sum.Integers[2],
			Type:      sum.Integers[3],
			StartAddr: sum.Integers[4],
			EndAddr:   sum.Integers[5],
			StartET:   sum.Doubles[0],
			EndET:     sum.Doubles[1],
		})
	}

	// Evaluate Target 3 (Earth Barycenter) at ET=0 (J2000 epoch)
	et := 0.0
	seg, err := spk.SelectSegment(segments, 3, et)
	testutil.AssertNoError(t, err)

	pos, vel, err := spk.EvaluateSegment(seg, r, et)
	if err != nil {
		t.Fatalf("EvaluateSegment failed: %v", err)
	}

	if pos.Norm() == 0 || vel.Norm() == 0 {
		t.Error("evaluated position/velocity is exactly zero, expected realistic barycenter values")
	}
}

func TestEvalChebyshev(t *testing.T) {
	coeffs := []float64{1.0, 2.0, 3.0}

	// Math check for Degree 2 Chebyshev:
	// T0(x)=1, T1(x)=x, T2(x)=2x^2 - 1
	// p(x) = c0*T0 + c1*T1 + c2*T2
	// p(0.5) = 1(1) + 2(0.5) + 3(2*(0.25)-1) = 2 - 1.5 = 0.5

	// Derivatives w.r.t x:
	// T0'=0, T1'=1, T2'=4x
	// v(0.5) = 2(1) + 3(4*0.5) = 8
	p, v := spk.EvalChebyshev(coeffs, 0.5, 1.0, true)

	testutil.AssertNear(t, "Chebyshev Position", p, 0.5, 1e-6)
	testutil.AssertNear(t, "Chebyshev Velocity", v, 8.0, 1e-6)
}
