package spk_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
	"github.com/TuSKan/astrogo/vector"
)

// The record cache's contract (#802): a cached evaluation is the uncached one
// to the last bit, fails exactly as it does, reads each directory and record
// once, allocates nothing on a hit, and is safe for concurrent readers.
//
// Every fixture here has several records with every coefficient different, so
// an evaluation served from the wrong record could not come out right.

// chebyshevSegment is a type 2 or 3 segment of nRecs records, 10 s each from
// ET 0, with four coefficients per component.
func chebyshevSegment(segType int32, nRecs int) ([]float64, *spk.Segment) {
	components := 3
	if segType == 3 {
		components = 6
	}

	const nc = 4

	words := make([]float64, 0, (2+components*nc)*nRecs+4)

	for i := range nRecs {
		words = append(words, 10*float64(i)+5, 5) // mid, radius
		for c := range components * nc {
			words = append(words, 1e3*math.Sin(float64(97*i+13*c+1)))
		}
	}

	words = append(words, 0, 10, float64(2+components*nc), float64(nRecs)) // INIT, INTLEN, RSIZE, N

	return words, &spk.Segment{Type: segType, StartAddr: 1, EndAddr: int32(len(words)), StartET: 0, EndET: 10 * float64(nRecs)}
}

// type21Segment is a type 21 segment of nRecs records with MAXDIM 3, record i
// valid until ET 100·(i+1).
func type21Segment(nRecs int) ([]float64, *spk.Segment) {
	const maxDim = 3

	words := make([]float64, 0, (4*maxDim+11)*nRecs+nRecs+2)

	for i := range nRecs {
		f := float64(i)
		words = append(words, 100*f)                              // t0
		words = append(words, 10+f, 20+f, 30+f)                   // g
		words = append(words, 1e4+f, 1+f, 2e4+f, 2+f, 3e4+f, 3+f) // refPos/refVel interleaved

		for c := range 3 * maxDim { // the difference table
			words = append(words, 1e-3*math.Cos(float64(31*i+7*c+1)))
		}

		words = append(words, maxDim+1, maxDim, maxDim, maxDim) // KQMAX1, KQ
	}

	for i := range nRecs {
		words = append(words, 100*float64(i+1)) // the epoch table
	}

	words = append(words, maxDim, float64(nRecs))

	return words, &spk.Segment{Type: 21, StartAddr: 1, EndAddr: int32(len(words)), StartET: 0, EndET: 100 * float64(nRecs)}
}

// readers is an uncached and a cached Reader over the same words.
func readers(words []float64) (uncached, cached *spk.Reader) {
	uncached = newWordBuffer(words)

	return uncached, spk.WithRecordCache(uncached)
}

// sweep is epochs across seg in order, its record boundaries and final
// instant included, then the same epochs shuffled.
func sweep(seg *spk.Segment, step float64) []float64 {
	var ets []float64

	for et := seg.StartET; et <= seg.EndET; et += step {
		ets = append(ets, et)
	}

	ets = append(ets, seg.EndET)

	shuffled := append([]float64(nil), ets...)
	rand.New(rand.NewPCG(3, 5)).Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })

	return append(ets, shuffled...)
}

type fixture struct {
	name  string
	words []float64
	seg   *spk.Segment
	step  float64
}

func fixtures() []fixture {
	w2, s2 := chebyshevSegment(2, 6)
	w3, s3 := chebyshevSegment(3, 6)
	w21, s21 := type21Segment(5)

	return []fixture{
		{"type 2", w2, s2, 0.37},
		{"type 3", w3, s3, 0.37},
		{"type 21", w21, s21, 3.1},
	}
}

// TestRecordCacheIsBitIdentical: over each fixture, in order and shuffled,
// the cached evaluation is the uncached one to the last bit. The cached
// reader sees every hit and every miss the sweep produces.
func TestRecordCacheIsBitIdentical(t *testing.T) {
	for _, f := range fixtures() {
		uncached, cached := readers(f.words)

		for _, et := range sweep(f.seg, f.step) {
			wantP, wantV, wantErr := spk.EvaluateSegment(f.seg, uncached, et)
			gotP, gotV, gotErr := spk.EvaluateSegment(f.seg, cached, et)

			if wantErr != nil || gotErr != nil {
				t.Fatalf("%s at ET %v: errors uncached %v, cached %v", f.name, et, wantErr, gotErr)
			}

			if !sameVec(gotP, wantP) || !sameVec(gotV, wantV) {
				t.Fatalf("%s at ET %v: cached %+v %+v, uncached %+v %+v", f.name, et, gotP, gotV, wantP, wantV)
			}
		}
	}
}

func sameVec(a, b vector.Vec3) bool {
	return math.Float64bits(a.X) == math.Float64bits(b.X) &&
		math.Float64bits(a.Y) == math.Float64bits(b.Y) &&
		math.Float64bits(a.Z) == math.Float64bits(b.Z)
}

// TestRecordCacheFailsAsUncachedDoes: corrupt data fails the same way cached,
// on every call — nothing invalid is kept — and an evaluation in a good
// record still succeeds after one in a bad record has failed.
func TestRecordCacheFailsAsUncachedDoes(t *testing.T) {
	corrupt := func(words []float64, edit func([]float64)) []float64 {
		w := append([]float64(nil), words...)
		edit(w)

		return w
	}

	w2, s2 := chebyshevSegment(2, 3)
	w21, s21 := type21Segment(3)

	rec2 := 2 + 3*4 // a type 2 fixture record's words

	for _, c := range []struct {
		name  string
		words []float64
		seg   *spk.Segment
		et    float64
		want  error
	}{
		{"type 2, N = 0", corrupt(w2, func(w []float64) { w[len(w)-1] = 0 }), s2, 5, spk.ErrInvalidRecordCount},
		{"type 2, INTLEN NaN", corrupt(w2, func(w []float64) { w[len(w)-3] = math.NaN() }), s2, 5, spk.ErrCorruptSPK},
		{"type 2, an epoch a record past the end", w2, s2, 41, spk.ErrCorruptSPK},
		{"type 2, record 1's radius zero", corrupt(w2, func(w []float64) { w[rec2+1] = 0 }), s2, 15, spk.ErrCorruptSPK},
		{"type 21, MAXDIM 0", corrupt(w21, func(w []float64) { w[len(w)-2] = 0 }), s21, 50, spk.ErrInvalidOrder},
		{"type 21, record 1's KQMAX1 NaN", corrupt(w21, func(w []float64) { w[(4*3+11)+4*3+7] = math.NaN() }), s21, 150, spk.ErrCorruptSPK},
	} {
		uncached, cached := readers(c.words)

		_, _, want := spk.EvaluateSegment(c.seg, uncached, c.et)
		if !errors.Is(want, c.want) {
			t.Fatalf("%s: the fixture itself: err = %v, want %v", c.name, want, c.want)
		}

		for call := range 3 {
			if _, _, got := spk.EvaluateSegment(c.seg, cached, c.et); got == nil || got.Error() != want.Error() {
				t.Errorf("%s, call %d: cached err = %v, uncached %v", c.name, call+1, got, want)
			}
		}
	}

	// A good record evaluated, a bad one refused, the good one again: the
	// failure must neither poison the entry nor be served from it.
	words := corrupt(w2, func(w []float64) { w[rec2+1] = 0 })
	_, cached := readers(words)

	for _, step := range []struct {
		et      float64
		wantErr bool
	}{{5, false}, {15, true}, {15, true}, {5, false}, {25, false}, {15, true}} {
		_, _, err := spk.EvaluateSegment(s2, cached, step.et)
		if (err != nil) != step.wantErr {
			t.Errorf("ET %v: err = %v, want error %v", step.et, err, step.wantErr)
		}
	}
}

// countingFile counts ReadAt calls.
type countingFile struct {
	fakeReaderAt

	reads atomic.Int64
}

func (f *countingFile) ReadAt(p []byte, off int64) (int, error) {
	f.reads.Add(1)

	return f.fakeReaderAt.ReadAt(p, off) //nolint:wrapcheck // a pass-through
}

// TestRecordCacheReadsOnce: a segment's directory is read once, and a record
// once for as long as evaluations stay in it — the reads the cache exists to
// remove, counted.
func TestRecordCacheReadsOnce(t *testing.T) {
	for _, f := range fixtures() {
		file := &countingFile{fakeReaderAt: newWordBuffer(f.words).F.(fakeReaderAt)} //nolint:forcetypeassert // newWordBuffer's own type
		cached := spk.WithRecordCache(&spk.Reader{F: file, FileRec: spk.FileRecord{Order: binary.LittleEndian}})

		evaluate := func(et float64) int64 {
			before := file.reads.Load()

			if _, _, err := spk.EvaluateSegment(f.seg, cached, et); err != nil {
				t.Fatalf("%s at ET %v: %v", f.name, et, err)
			}

			return file.reads.Load() - before
		}

		mid := f.seg.StartET + 0.5*(f.seg.EndET-f.seg.StartET)/5 // inside the first record

		// The directory is one read for types 2 and 3; type 21's is two, its
		// MAXDIM and count and then its epoch table.
		first := int64(2)
		if f.seg.Type == 21 {
			first = 3
		}

		if got := evaluate(f.seg.StartET + 1); got != first {
			t.Errorf("%s: a first evaluation read %d times, want %d (directory, record)", f.name, got, first)
		}

		if got := evaluate(mid); got != 0 {
			t.Errorf("%s: an evaluation in the same record read %d times, want 0", f.name, got)
		}

		if got := evaluate(f.seg.EndET); got != 1 {
			t.Errorf("%s: an evaluation in a new record read %d times, want 1 (the record)", f.name, got)
		}
	}
}

// TestRecordCacheHitAllocatesNothing is an allocation contract: an
// evaluation the cache answers performs no read, so it has nothing to
// allocate. Before #802 each one allocated four times (type 2/3) or six
// (type 21).
func TestRecordCacheHitAllocatesNothing(t *testing.T) {
	for _, f := range fixtures() {
		_, cached := readers(f.words)
		et := f.seg.StartET + 1

		if _, _, err := spk.EvaluateSegment(f.seg, cached, et); err != nil {
			t.Fatalf("%s: %v", f.name, err)
		}

		if got := testing.AllocsPerRun(200, func() {
			_, _, _ = spk.EvaluateSegment(f.seg, cached, et)
		}); got != 0 {
			t.Errorf("%s: a cached evaluation allocates %v times, want 0", f.name, got)
		}
	}
}

// TestRecordCacheSharedByConcurrentReaders: sixteen goroutines evaluating
// random epochs through one cached Reader each get the uncached answer. The
// race detector, in CI's race job, checks the sharing itself.
func TestRecordCacheSharedByConcurrentReaders(t *testing.T) {
	for _, f := range fixtures() {
		uncached, cached := readers(f.words)
		ets := sweep(f.seg, f.step)

		want := make([][2]vector.Vec3, len(ets))
		for i, et := range ets {
			p, v, err := spk.EvaluateSegment(f.seg, uncached, et)
			if err != nil {
				t.Fatalf("%s at ET %v: %v", f.name, et, err)
			}

			want[i] = [2]vector.Vec3{p, v}
		}

		var wg sync.WaitGroup

		var wrong atomic.Int64

		for g := range 16 {
			wg.Go(func() {
				r := rand.New(rand.NewPCG(uint64(g), 9))

				for range 2000 {
					i := r.IntN(len(ets))

					p, v, err := spk.EvaluateSegment(f.seg, cached, ets[i])
					if err != nil || !sameVec(p, want[i][0]) || !sameVec(v, want[i][1]) {
						wrong.Add(1)
					}
				}
			})
		}

		wg.Wait()

		if n := wrong.Load(); n != 0 {
			t.Errorf("%s: %d of 32000 concurrent evaluations differ from the uncached answer", f.name, n)
		}
	}
}

// closingFile fails every read once closed, as a real file does.
type closingFile struct {
	*bytes.Reader

	closed atomic.Bool
}

var errClosed = errors.New("closed")

func (f *closingFile) ReadAt(p []byte, off int64) (int, error) {
	if f.closed.Load() {
		return 0, errClosed
	}

	return f.Reader.ReadAt(p, off) //nolint:wrapcheck // a pass-through
}

func (f *closingFile) Close() error {
	f.closed.Store(true)

	return nil
}

// TestClosedReaderAnswersNothingCached: before the cache, an evaluation on a
// closed Reader always needed a read, and failed. Close drops the cache so
// that remains true.
func TestClosedReaderAnswersNothingCached(t *testing.T) {
	words, seg := chebyshevSegment(2, 3)

	buf := newWordBuffer(words).F.(fakeReaderAt) //nolint:forcetypeassert // newWordBuffer's own type
	file := &closingFile{Reader: buf.Reader}
	cached := spk.WithRecordCache(&spk.Reader{F: file, FileRec: spk.FileRecord{Order: binary.LittleEndian}})

	if _, _, err := spk.EvaluateSegment(seg, cached, 5); err != nil {
		t.Fatalf("before Close: %v", err)
	}

	if err := cached.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, _, err := spk.EvaluateSegment(seg, cached, 5); !errors.Is(err, errClosed) {
		t.Errorf("after Close, the cached record's evaluation: err = %v, want the read's own failure", err)
	}
}
