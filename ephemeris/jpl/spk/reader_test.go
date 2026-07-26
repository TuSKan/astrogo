package spk_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
	"github.com/TuSKan/astrogo/internal/testutil"
)

// fakeReaderAt adapts a *bytes.Reader (which already implements io.ReaderAt)
// into spk.ReadAtCloser for a synthetic in-memory segment, with no real file
// to close.
type fakeReaderAt struct{ *bytes.Reader }

func (fakeReaderAt) Close() error { return nil }

// newWordBuffer packs vals as consecutive little-endian float64 DAF words
// (1-based addressing, matching Reader.ReadDoubles) into a *spk.Reader.
func newWordBuffer(vals []float64) *spk.Reader {
	buf := make([]byte, len(vals)*8)
	for i, v := range vals {
		binary.LittleEndian.PutUint64(buf[i*8:], math.Float64bits(v))
	}

	return &spk.Reader{
		F:       fakeReaderAt{bytes.NewReader(buf)},
		FileRec: spk.FileRecord{Order: binary.LittleEndian},
	}
}

func TestSPKReader(t *testing.T) {
	// Bootstrap the download process robustly via the provider logic
	prov, err := jpl.NewProvider(context.Background(), core.Planets, "de440s")
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

	segments := make([]spk.Segment, 0, len(summaries))
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

// TestEvaluateType21 is a regression test for a bug where the type 21
// (Extended Modified Difference Array) reader assumed a fixed MAXDIM=15 and
// a block-grouped [px,py,pz,vx,vy,vz] record layout. Real records store a
// per-segment MAXDIM and interleave reference position/velocity per axis
// ([px,vx,py,vy,pz,vz]) — confirmed against a real Horizons-generated
// small-body kernel, where the old layout silently mixed a velocity
// component (km/s) into a position component (km), collapsing the derived
// heliocentric distance for every real asteroid/comet to a physically
// impossible near-zero value.
//
// This builds a single-record, MAXDIM=1 synthetic segment by hand (no
// network access) with KQ=0 on every axis, so the modified-difference
// table itself never contributes (isolating the record-layout decode from
// the FC/WC/W recursion), and checks two epochs: et==t0 (delta=0, must
// return refPos/refVel exactly regardless of layout) and a later epoch
// (must return refPos + delta*refVel / refVel — plain linear motion).
func TestEvaluateType21(t *testing.T) {
	const (
		t0        = 100.0
		maxDim    = 1.0
		kqmax1    = 2.0 // must be >= 2
		startAddr = 1
	)

	refPos := [3]float64{10, 20, 30}
	refVel := [3]float64{1, 2, 3}

	// Word layout (1-based, matching the doc comment on evaluateType21):
	// t0 | G(1) | px,vx,py,vy,pz,vz (interleaved) | dt(3, unused since KQ=0) | kqmax1 | kq[3]
	rec := []float64{
		t0, 1.0,
		refPos[0], refVel[0], refPos[1], refVel[1], refPos[2], refVel[2],
		99, 99, 99, // DT, garbage — must never be read since KQ=0 everywhere
		kqmax1,
		0, 0, 0, // KQ
	}

	words := append(append([]float64{}, rec...),
		999999.0, // epoch table (1 entry; irrelevant with nRecs=1)
		maxDim,   // MAXDIM at EndAddr-1
		1.0,      // nRecs at EndAddr
	)

	r := newWordBuffer(words)
	seg := &spk.Segment{
		Type:      21,
		StartAddr: startAddr,
		EndAddr:   int32(len(words)),
		StartET:   t0,
		EndET:     t0 + 1000,
	}

	// delta == 0: must return refPos/refVel exactly, regardless of layout.
	pos, vel, err := spk.EvaluateSegment(seg, r, t0)
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "pos.X @ delta=0", pos.X, refPos[0], 1e-9)
	testutil.AssertNear(t, "pos.Y @ delta=0", pos.Y, refPos[1], 1e-9)
	testutil.AssertNear(t, "pos.Z @ delta=0", pos.Z, refPos[2], 1e-9)
	testutil.AssertNear(t, "vel.X @ delta=0", vel.X, refVel[0], 1e-9)
	testutil.AssertNear(t, "vel.Y @ delta=0", vel.Y, refVel[1], 1e-9)
	testutil.AssertNear(t, "vel.Z @ delta=0", vel.Z, refVel[2], 1e-9)

	// delta == 5: KQ=0 means no MDA-table correction, so this is plain
	// linear motion: pos = refPos + delta*refVel, vel = refVel.
	const delta = 5.0

	pos, vel, err = spk.EvaluateSegment(seg, r, t0+delta)
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "pos.X @ delta=5", pos.X, refPos[0]+delta*refVel[0], 1e-9)
	testutil.AssertNear(t, "pos.Y @ delta=5", pos.Y, refPos[1]+delta*refVel[1], 1e-9)
	testutil.AssertNear(t, "pos.Z @ delta=5", pos.Z, refPos[2]+delta*refVel[2], 1e-9)
	testutil.AssertNear(t, "vel.X @ delta=5", vel.X, refVel[0], 1e-9)
	testutil.AssertNear(t, "vel.Y @ delta=5", vel.Y, refVel[1], 1e-9)
	testutil.AssertNear(t, "vel.Z @ delta=5", vel.Z, refVel[2], 1e-9)
}
