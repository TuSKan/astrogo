package spk_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
)

// This file deterministically exercises every validation branch added to
// harden the SPK/DAF reader against corrupted/adversarial kernels (see
// fuzz_test.go for the property-based counterpart — "never panics" — this
// file instead pins down "rejects this specific malformed input with this
// specific sentinel error").

func pokeFloat64(buf []byte, offset int, v float64, order binary.ByteOrder) {
	order.PutUint64(buf[offset:offset+8], math.Float64bits(v))
}

func newReaderWithSummaryData(data []byte, nd, ni, fwd int32) *spk.Reader {
	return &spk.Reader{
		F:       fakeReaderAt{bytes.NewReader(data)},
		FileRec: spk.FileRecord{ND: nd, NI: ni, FWD: fwd, Order: binary.LittleEndian},
	}
}

func TestReadSummaries_RejectsInvalidShape(t *testing.T) {
	cases := []struct {
		name   string
		nd, ni int32
	}{
		{"negative ND", -1, 6},
		{"negative NI", 2, -1},
		{"both zero", 0, 0},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			r := newReaderWithSummaryData(nil, tt.nd, tt.ni, 0)

			_, err := r.ReadSummaries()
			if !errors.Is(err, spk.ErrCorruptSPK) {
				t.Errorf("err = %v, want ErrCorruptSPK", err)
			}
		})
	}
}

func TestReadSummaries_RejectsOversizedSummary(t *testing.T) {
	// ND/NI large enough that a single summary entry couldn't fit in one
	// 1024-byte directory record.
	r := newReaderWithSummaryData(nil, 1000, 1000, 0)

	_, err := r.ReadSummaries()
	if !errors.Is(err, spk.ErrCorruptSPK) {
		t.Errorf("err = %v, want ErrCorruptSPK", err)
	}
}

func TestReadSummaries_RejectsNegativeFWD(t *testing.T) {
	r := newReaderWithSummaryData(nil, 2, 6, -5)

	_, err := r.ReadSummaries()
	if !errors.Is(err, spk.ErrCorruptSPK) {
		t.Errorf("err = %v, want ErrCorruptSPK", err)
	}
}

func TestReadSummaries_RejectsCyclicFWD(t *testing.T) {
	order := binary.LittleEndian
	data := make([]byte, spk.RecordSize)
	pokeFloat64(data, 0, 1.0, order) // FWD points back to record 1 (itself)
	pokeFloat64(data, 16, 0.0, order)

	r := newReaderWithSummaryData(data, 2, 6, 1)

	_, err := r.ReadSummaries()
	if !errors.Is(err, spk.ErrCorruptSPK) {
		t.Errorf("err = %v, want ErrCorruptSPK", err)
	}
}

func TestReadSummaries_RejectsNonFiniteFWD(t *testing.T) {
	order := binary.LittleEndian
	data := make([]byte, spk.RecordSize)
	pokeFloat64(data, 0, math.NaN(), order)
	pokeFloat64(data, 16, 0.0, order)

	r := newReaderWithSummaryData(data, 2, 6, 1)

	_, err := r.ReadSummaries()
	if !errors.Is(err, spk.ErrCorruptSPK) {
		t.Errorf("err = %v, want ErrCorruptSPK", err)
	}
}

func TestReadSummaries_RejectsInvalidSummaryCount(t *testing.T) {
	cases := []struct {
		name string
		nSum float64
	}{
		{"negative", -1.0},
		{"non-finite", math.NaN()},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			order := binary.LittleEndian
			data := make([]byte, spk.RecordSize)
			pokeFloat64(data, 0, 0.0, order)
			pokeFloat64(data, 16, tt.nSum, order)

			r := newReaderWithSummaryData(data, 2, 6, 1)

			_, err := r.ReadSummaries()
			if !errors.Is(err, spk.ErrCorruptSPK) {
				t.Errorf("err = %v, want ErrCorruptSPK", err)
			}
		})
	}
}

func TestReadSummaries_RejectsSummaryCountOverflowingRecord(t *testing.T) {
	order := binary.LittleEndian
	data := make([]byte, spk.RecordSize)
	pokeFloat64(data, 0, 0.0, order)
	pokeFloat64(data, 16, 1000.0, order) // 1000 summaries of 40 bytes each: way over 1024

	r := newReaderWithSummaryData(data, 2, 6, 1)

	_, err := r.ReadSummaries()
	if !errors.Is(err, spk.ErrCorruptSPK) {
		t.Errorf("err = %v, want ErrCorruptSPK", err)
	}
}

func TestReadSummaries_ValidSingleRecord(t *testing.T) {
	order := binary.LittleEndian
	data := buildSummaryRecordSPK(order, 0, -1e8, 1e8, 3, 10, 1, 2, 100, 200)

	r := newReaderWithSummaryData(data, 2, 6, 1)

	sums, err := r.ReadSummaries()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(sums) != 1 {
		t.Fatalf("len(sums) = %d, want 1", len(sums))
	}

	if sums[0].Integers[0] != 3 {
		t.Errorf("target = %d, want 3", sums[0].Integers[0])
	}
}

func TestReadDoubles_RejectsNonPositiveCount(t *testing.T) {
	r := &spk.Reader{F: fakeReaderAt{bytes.NewReader(nil)}, FileRec: spk.FileRecord{Order: binary.LittleEndian}}

	_, err := r.ReadDoubles(5, 1) // endWord < startWord

	if !errors.Is(err, spk.ErrInvalidWordBounds) {
		t.Errorf("err = %v, want ErrInvalidWordBounds", err)
	}
}

func TestReadDoubles_RejectsExcessiveCount(t *testing.T) {
	r := &spk.Reader{F: fakeReaderAt{bytes.NewReader(nil)}, FileRec: spk.FileRecord{Order: binary.LittleEndian}}

	_, err := r.ReadDoubles(1, 1<<28) // far beyond maxDoublesPerRead

	if !errors.Is(err, spk.ErrInvalidWordBounds) {
		t.Errorf("err = %v, want ErrInvalidWordBounds", err)
	}
}

func TestReadDoubles_RejectsUnexpectedEOF(t *testing.T) {
	r := &spk.Reader{F: fakeReaderAt{bytes.NewReader(make([]byte, 8))}, FileRec: spk.FileRecord{Order: binary.LittleEndian}}

	_, err := r.ReadDoubles(1, 5) // requests 5 words, only 1 word of data backing it

	if !errors.Is(err, spk.ErrCorruptSPK) {
		t.Errorf("err = %v, want ErrCorruptSPK", err)
	}
}

// buildType21Words assembles a single-record type-21 segment word array
// with the given maxDim/kqmax1/kq/g, for exercising evaluateType21's
// validation branches deterministically. et is chosen so the record's own
// epoch entry is selected.
func buildType21Words(maxDim, kqmax1 int32, kq [3]float64, g []float64) []float64 {
	words := make([]float64, 0, 1+int(maxDim)+6+3*int(maxDim)+1+3+3)
	words = append(words, 0.0) // t0
	words = append(words, g...)
	words = append(words, 10, 1, 20, 2, 30, 3) // refPos/refVel interleaved
	words = append(words, make([]float64, 3*maxDim)...)
	words = append(words, float64(kqmax1))
	words = append(words, kq[:]...)
	words = append(words, 999999.0, float64(maxDim), 1.0) // epoch table, MAXDIM, nRecs

	return words
}

// evaluateType21Segment evaluates words at et=5.0 (t0 is always 0 in
// buildType21Words, so delta=5 exercises the FC/WC/W recursion every case
// here needs) and wraps EvaluateSegment's error for wrapcheck.
func evaluateType21Segment(words []float64) error {
	r := newWordBuffer(words)
	seg := &spk.Segment{Type: 21, StartAddr: 1, EndAddr: int32(len(words)), StartET: -1e9, EndET: 1e9}

	if _, _, err := spk.EvaluateSegment(seg, r, 5.0); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}

func TestEvaluateType21_RejectsInvalidMetadata(t *testing.T) {
	g1 := []float64{1.0}

	cases := []struct {
		name      string
		maxDim    int32
		kqmax1    int32
		kq        [3]float64
		g         []float64
		wantErrIs error
	}{
		{"zero MAXDIM", 0, 2, [3]float64{0, 0, 0}, nil, spk.ErrInvalidOrder},
		{"MAXDIM too large", 26, 2, [3]float64{0, 0, 0}, make([]float64, 26), spk.ErrInvalidOrder},
		{"KQMAX1 below range", 1, 1, [3]float64{0, 0, 0}, g1, spk.ErrInvalidOrder},
		{"KQMAX1 above range", 1, 27, [3]float64{0, 0, 0}, g1, spk.ErrInvalidOrder},
		{"KQMAX1 exceeds this segment's MAXDIM", 1, 3, [3]float64{0, 0, 0}, g1, spk.ErrInvalidOrder},
		{"KQ exceeds MAXDIM", 1, 2, [3]float64{2, 0, 0}, g1, spk.ErrInvalidOrder},
		{"negative KQ", 1, 2, [3]float64{-1, 0, 0}, g1, spk.ErrInvalidOrder},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			words := buildType21Words(tt.maxDim, tt.kqmax1, tt.kq, tt.g)

			err := evaluateType21Segment(words)
			if !errors.Is(err, tt.wantErrIs) {
				t.Errorf("err = %v, want %v", err, tt.wantErrIs)
			}
		})
	}
}

func TestEvaluateType21_RejectsNonFiniteRecordCount(t *testing.T) {
	words := buildType21Words(1, 2, [3]float64{0, 0, 0}, []float64{1.0})
	// Corrupt nRecs (last word) to NaN.
	words[len(words)-1] = math.NaN()

	err := evaluateType21Segment(words)
	if !errors.Is(err, spk.ErrCorruptSPK) {
		t.Errorf("err = %v, want ErrCorruptSPK", err)
	}
}

func TestEvaluateType21_RejectsInvalidRecordCount(t *testing.T) {
	cases := []struct {
		name  string
		nRecs float64
	}{
		{"zero", 0},
		{"negative", -1},
		{"too large", 20_000_000},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			words := buildType21Words(1, 2, [3]float64{0, 0, 0}, []float64{1.0})
			words[len(words)-1] = tt.nRecs

			err := evaluateType21Segment(words)
			if !errors.Is(err, spk.ErrInvalidRecordCount) {
				t.Errorf("err = %v, want ErrInvalidRecordCount", err)
			}
		})
	}
}

func TestEvaluateType21_RejectsNonFiniteKQMetadata(t *testing.T) {
	words := buildType21Words(1, 2, [3]float64{0, 0, 0}, []float64{1.0})
	// KQMAX1 word is right after the MDA table: t0(1) + g(maxDim=1) + refPosVel(6) + dt(3*maxDim=3) = index 11.
	words[11] = math.NaN()

	err := evaluateType21Segment(words)
	if !errors.Is(err, spk.ErrCorruptSPK) {
		t.Errorf("err = %v, want ErrCorruptSPK", err)
	}
}

func TestEvaluateType21_RejectsNonFiniteResult(t *testing.T) {
	// maxDim=2, kqmax1=3 puts the g-division loop (mq2=1) in play; g[0]=NaN
	// isn't caught by the "zero step size" check (NaN != 0) and propagates
	// through the FC/W recursion, landing in w[2] specifically — so KQ[0]
	// must be large enough (2) that axis 0's sum actually reads w[2], or
	// the contamination never reaches the returned position/velocity.
	words := buildType21Words(2, 3, [3]float64{2, 0, 0}, []float64{math.NaN(), 1.0})

	dtOffset := 1 + 2 + 6 // t0(1) + g(maxDim=2) + refPosVel(6)
	for i := range 6 {
		words[dtOffset+i] = 1.0 // dt table, all axes: non-zero so axisSum actually uses w
	}

	err := evaluateType21Segment(words)
	if !errors.Is(err, spk.ErrCorruptSPK) {
		t.Errorf("err = %v, want ErrCorruptSPK", err)
	}
}

func TestEvaluateType21_ValidRecord(t *testing.T) {
	// Regression guard: the hardening above must not reject a legitimate
	// record (mirrors TestEvaluateType21 in reader_test.go, at delta=0).
	words := buildType21Words(1, 2, [3]float64{0, 0, 0}, []float64{1.0})

	r := newWordBuffer(words)
	seg := &spk.Segment{Type: 21, StartAddr: 1, EndAddr: int32(len(words)), StartET: -1e9, EndET: 1e9}

	pos, _, err := spk.EvaluateSegment(seg, r, 0.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pos.X != 10 || pos.Y != 20 || pos.Z != 30 {
		t.Errorf("pos = %+v, want refPos (10,20,30) at delta=0", pos)
	}
}

// buildType2Words assembles a single-record type-2 segment word array. The
// meta block evaluateType2 reads is ReadDoubles(EndAddr-3, EndAddr) — 4
// words, not 3 — even though only meta[0..2] (tInit, tLen, rSize) are
// used; the trailing 0.0 is the unused 4th word (nRecs, in a real kernel).
func buildType2Words(mid, radius float64, coeffs []float64) []float64 {
	words := []float64{mid, radius}
	words = append(words, coeffs...)
	words = append(words, coeffs...)
	words = append(words, coeffs...)

	rSize := float64(2 + 3*len(coeffs))

	return append(words, 0.0, 1.0, rSize, 0.0) // tInit, tLen, rSize, (unused)
}

func evaluateType2Segment(words []float64) error {
	r := newWordBuffer(words)
	seg := &spk.Segment{Type: 2, StartAddr: 1, EndAddr: int32(len(words)), StartET: -1e9, EndET: 1e9}

	if _, _, err := spk.EvaluateSegment(seg, r, 5.0); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}

func TestEvaluateType2_RejectsInvalidMeta(t *testing.T) {
	cases := []struct {
		name          string
		tInit, tLen   float64
		rSize         float64
		wantErrSubstr error
	}{
		{"non-finite tLen", 0, math.NaN(), 5, spk.ErrCorruptSPK},
		{"zero tLen", 0, 0, 5, spk.ErrCorruptSPK},
		{"negative tLen", 0, -1, 5, spk.ErrCorruptSPK},
		{"rSize too small", 0, 1, 1, spk.ErrRecordTooShort},
		{"rSize yields zero coefficients", 0, 1, 2, spk.ErrRecordTooShort},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			// The rec block itself is well-formed (matches buildType2Words'
			// own rSize=5 for a single coefficient) — only the trailing meta
			// triple is overridden, since every case here is expected to
			// fail validateChebyshevMeta/the nCoeffs<=0 check, both of which
			// run before rec is ever read.
			words := buildType2Words(0, 1, []float64{1.0})
			n := len(words)
			words[n-4] = tt.tInit
			words[n-3] = tt.tLen
			words[n-2] = tt.rSize

			err := evaluateType2Segment(words)
			if !errors.Is(err, tt.wantErrSubstr) {
				t.Errorf("err = %v, want %v", err, tt.wantErrSubstr)
			}
		})
	}
}

func TestEvaluateType2_RejectsInvalidMidRadius(t *testing.T) {
	cases := []struct {
		name        string
		mid, radius float64
	}{
		{"non-finite mid", math.NaN(), 1.0},
		{"non-finite radius", 0.0, math.NaN()},
		{"zero radius", 0.0, 0.0},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			words := buildType2Words(tt.mid, tt.radius, []float64{1.0})

			err := evaluateType2Segment(words)
			if !errors.Is(err, spk.ErrCorruptSPK) {
				t.Errorf("err = %v, want ErrCorruptSPK", err)
			}
		})
	}
}

func TestEvaluateType2_RejectsNonFiniteResult(t *testing.T) {
	words := buildType2Words(0.0, 1.0, []float64{math.NaN()})

	err := evaluateType2Segment(words)
	if !errors.Is(err, spk.ErrCorruptSPK) {
		t.Errorf("err = %v, want ErrCorruptSPK", err)
	}
}

func TestEvaluateType2_ValidRecord(t *testing.T) {
	words := buildType2Words(0.0, 1.0, []float64{7.0})

	r := newWordBuffer(words)
	seg := &spk.Segment{Type: 2, StartAddr: 1, EndAddr: int32(len(words)), StartET: -1e9, EndET: 1e9}

	pos, _, err := spk.EvaluateSegment(seg, r, 0.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pos.X != 7 || pos.Y != 7 || pos.Z != 7 {
		t.Errorf("pos = %+v, want (7,7,7) (single-coefficient Chebyshev is constant)", pos)
	}
}

// buildType3Words assembles a single-record type-3 segment word array — see
// buildType2Words' doc comment for why the meta block needs 4 trailing words.
func buildType3Words(mid, radius float64, coeff float64) []float64 {
	words := []float64{mid, radius}
	for range 6 { // pos x3, vel x3 axes, one coefficient each
		words = append(words, coeff)
	}

	return append(words, 0.0, 1.0, 8.0, 0.0) // tInit, tLen, rSize (2+6*1=8), (unused)
}

func evaluateType3Segment(words []float64) error {
	r := newWordBuffer(words)
	seg := &spk.Segment{Type: 3, StartAddr: 1, EndAddr: int32(len(words)), StartET: -1e9, EndET: 1e9}

	if _, _, err := spk.EvaluateSegment(seg, r, 5.0); err != nil {
		return fmt.Errorf("%w", err)
	}

	return nil
}

func TestEvaluateType3_RejectsInvalidMidRadius(t *testing.T) {
	words := buildType3Words(math.NaN(), 1.0, 1.0)

	err := evaluateType3Segment(words)
	if !errors.Is(err, spk.ErrCorruptSPK) {
		t.Errorf("err = %v, want ErrCorruptSPK", err)
	}
}

func TestEvaluateType3_RejectsNonFiniteResult(t *testing.T) {
	words := buildType3Words(0.0, 1.0, math.NaN())

	err := evaluateType3Segment(words)
	if !errors.Is(err, spk.ErrCorruptSPK) {
		t.Errorf("err = %v, want ErrCorruptSPK", err)
	}
}

func TestEvaluateType3_ValidRecord(t *testing.T) {
	words := buildType3Words(0.0, 1.0, 9.0)

	r := newWordBuffer(words)
	seg := &spk.Segment{Type: 3, StartAddr: 1, EndAddr: int32(len(words)), StartET: -1e9, EndET: 1e9}

	pos, vel, err := spk.EvaluateSegment(seg, r, 0.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pos.X != 9 || vel.X != 9 {
		t.Errorf("pos.X/vel.X = %v/%v, want 9/9 (single-coefficient Chebyshev is constant)", pos.X, vel.X)
	}
}

func TestEvaluateSegment_UnsupportedType(t *testing.T) {
	r := newWordBuffer([]float64{0})
	seg := &spk.Segment{Type: 99, StartAddr: 1, EndAddr: 1}

	_, _, err := spk.EvaluateSegment(seg, r, 0.0)
	if !errors.Is(err, spk.ErrUnsupportedSegment) {
		t.Errorf("err = %v, want ErrUnsupportedSegment", err)
	}
}
