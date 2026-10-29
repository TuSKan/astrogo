package kepler

import (
	"errors"
	"math"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/internal/gofaext"
)

// The memo's contract is that it changes how often Epv00 is evaluated and
// nothing about what it returns. Every test here therefore compares against a
// direct call of gofaext.Epv00 by the bits of its answer, never by a tolerance:
// a memo that returned a neighbor's value, or a stale one after an eviction,
// would pass a tolerance and fail this.

// epv00Direct is Epv00 at d1 + d2 with no memo in the way.
func epv00Direct(tb testing.TB, d1, d2 float64) epv00At {
	tb.Helper()

	pvh, pvb, status := gofaext.Epv00(d1, d2)
	if status < 0 {
		tb.Fatalf("Epv00(%v, %v) status %d", d1, d2, status)
	}

	return epv00At{set: true, d1: d1, d2: d2, pvh: pvh, pvb: pvb}
}

// bitsEqual is whether a and b are the same to the last bit in every field. It
// is not ==, which calls -0 and +0 equal and a NaN unequal to itself.
func bitsEqual(a, b epv00At) bool {
	if a.set != b.set ||
		math.Float64bits(a.d1) != math.Float64bits(b.d1) ||
		math.Float64bits(a.d2) != math.Float64bits(b.d2) {
		return false
	}

	for i := range 2 {
		for j := range 3 {
			if math.Float64bits(a.pvh[i][j]) != math.Float64bits(b.pvh[i][j]) ||
				math.Float64bits(a.pvb[i][j]) != math.Float64bits(b.pvb[i][j]) {
				return false
			}
		}
	}

	return true
}

// sameBits fails unless got is want to the last bit, in every field.
func sameBits(tb testing.TB, got, want epv00At) {
	tb.Helper()

	if !bitsEqual(got, want) {
		tb.Fatalf("at (%v, %v): got %+v, want %+v", want.d1, want.d2, got, want)
	}
}

// epv00Key is a TDB date within 2020 to 2030, split the way Time.JDParts splits
// one: the day, and the fraction of it.
func epv00Key(r *rand.Rand) (d1, d2 float64) {
	day := 2458849.0 + math.Floor(r.Float64()*3653)

	return day, r.Float64()
}

// epv00Requests is n requests that revisit earlier ones at every distance a
// search does: back to back (the Sun and the Earth at one epoch), a few
// instants back (a bisection), and far back (the next pass over a window).
// Deterministic, so a failure reproduces.
func epv00Requests(n int) [][2]float64 {
	r := rand.New(rand.NewPCG(7, 11))
	out := make([][2]float64, 0, n)

	for len(out) < n {
		if len(out) > 0 && r.Float64() < 0.6 {
			back := 1 + int(math.Exp(r.Float64()*math.Log(float64(len(out)))))
			out = append(out, out[max(0, len(out)-back)])

			continue
		}

		d1, d2 := epv00Key(r)
		out = append(out, [2]float64{d1, d2})
	}

	return out
}

// countingEval is gofaext.Epv00 that counts how often it was asked.
type countingEval struct{ n atomic.Int64 }

func (c *countingEval) eval(d1, d2 float64) (pvh, pvb [2][3]float64, status int) {
	c.n.Add(1)

	return gofaext.Epv00(d1, d2)
}

func (m *epv00Memo) entries() int {
	n := 0

	for i := range m.shards {
		sh := &m.shards[i]
		sh.mu.Lock()

		for j := range sh.sets {
			for k := range sh.sets[j].ways {
				if sh.sets[j].ways[k].set {
					n++
				}
			}
		}

		sh.mu.Unlock()
	}

	return n
}

// allocated is how many of the memo's shards have a table.
func (m *epv00Memo) allocated() int {
	n := 0

	for i := range m.shards {
		sh := &m.shards[i]
		sh.mu.Lock()

		if sh.sets != nil {
			n++
		}

		sh.mu.Unlock()
	}

	return n
}

// TestEpv00MemoIsBitIdentical is the claim the memo is allowed to make: over
// requests that hit, miss, evict and grow the table, every answer is the one a
// direct Epv00 gives.
func TestEpv00MemoIsBitIdentical(t *testing.T) {
	s := &sofaBase{memo: &epv00Memo{}}

	for _, req := range epv00Requests(30000) {
		got, err := s.epv00(req[0], req[1])
		if err != nil {
			t.Fatal(err)
		}

		sameBits(t, got, epv00Direct(t, req[0], req[1]))
	}
}

// TestEpv00MemoComputesAnInstantOnce is what the memo is for: asking again for
// an instant already held does not evaluate the series again.
func TestEpv00MemoComputesAnInstantOnce(t *testing.T) {
	// Four instants fit one set of any table, so none can be evicted by the
	// others whatever they hash to.
	var c countingEval

	s := &sofaBase{memo: &epv00Memo{}, eval: c.eval}
	keys := [][2]float64{{2461000, 0.1}, {2461000, 0.2}, {2461001, 0.3}, {2461002, 0.4}}

	for range 10 {
		for _, k := range keys {
			if _, err := s.epv00(k[0], k[1]); err != nil {
				t.Fatal(err)
			}
		}
	}

	if got := c.n.Load(); got != int64(len(keys)) {
		t.Errorf("%d evaluations for %d instants asked 10 times each, want %d", got, len(keys), len(keys))
	}
}

// TestEpv00MemoHoldsAWorkingSet is the case the one-slot memo it replaces
// missed: a search that comes back to a few hundred instants after visiting
// others in between.
func TestEpv00MemoHoldsAWorkingSet(t *testing.T) {
	var c countingEval

	s := &sofaBase{memo: &epv00Memo{}, eval: c.eval}
	r := rand.New(rand.NewPCG(3, 5))

	const distinct = 300

	keys := make([][2]float64, distinct)
	for i := range keys {
		keys[i][0], keys[i][1] = epv00Key(r)
	}

	const passes = 5

	for range passes {
		for _, k := range keys {
			if _, err := s.epv00(k[0], k[1]); err != nil {
				t.Fatal(err)
			}
		}
	}

	// A set-associative table can lose a few to conflicts, so this is a bound
	// and not an equality; the one-slot memo evaluated all passes*distinct.
	if got, limit := c.n.Load(), int64(distinct*125/100); got > limit {
		t.Errorf("%d evaluations for %d instants over %d passes, want at most %d", got, distinct, passes, limit)
	}
}

// TestEpv00MemoIsBounded: however many instants are asked, the memo holds no
// more than its cap.
func TestEpv00MemoIsBounded(t *testing.T) {
	m := &epv00Memo{}
	s := &sofaBase{memo: m}
	r := rand.New(rand.NewPCG(13, 17))

	const limit = epv00Shards * epv00ShardSets * epv00Ways

	for range 3 * limit {
		d1, d2 := epv00Key(r)
		if _, err := s.epv00(d1, d2); err != nil {
			t.Fatal(err)
		}
	}

	if got := m.entries(); got > limit {
		t.Errorf("memo holds %d entries, want at most %d", got, limit)
	}
}

// TestEpv00MemoAllocatesOnDemand: a program that evaluates Epv00 once should
// not pay for the whole table, only for the shard the instant falls in.
func TestEpv00MemoAllocatesOnDemand(t *testing.T) {
	m := &epv00Memo{}
	s := &sofaBase{memo: m}

	if got := m.allocated(); got != 0 {
		t.Fatalf("a new memo has %d shards allocated before any request", got)
	}

	if _, err := s.epv00(2461000, 0.5); err != nil {
		t.Fatal(err)
	}

	if got := m.allocated(); got != 1 {
		t.Errorf("one request allocated %d shards, want 1", got)
	}
}

// TestEpv00MemoIsSharedAcrossProviders is why the memo is process-wide. A
// provider is built per call in places (eph.Default() in a constraint check),
// so a memo each held would start empty every time; the date a first provider
// evaluated is the date a second finds.
func TestEpv00MemoIsSharedAcrossProviders(t *testing.T) {
	var c countingEval

	// A date nothing else in this package asks for, since the memo is shared
	// with every test that leaves its sofaBase on the default.
	const d1, d2 = 2400000.5, 0.123456789

	first := &sofaBase{eval: c.eval}
	second := &sofaBase{eval: c.eval}

	a, err := first.epv00(d1, d2)
	if err != nil {
		t.Fatal(err)
	}

	b, err := second.epv00(d1, d2)
	if err != nil {
		t.Fatal(err)
	}

	if got := c.n.Load(); got != 1 {
		t.Errorf("%d evaluations for one date asked by two providers, want 1", got)
	}

	sameBits(t, b, a)
	sameBits(t, a, epv00Direct(t, d1, d2))
}

// TestEpv00MemoDoesNotKeepFailures: a failed evaluation is returned and never
// stored, so asking again asks the series again.
func TestEpv00MemoDoesNotKeepFailures(t *testing.T) {
	var calls atomic.Int64

	m := &epv00Memo{}

	s := &sofaBase{memo: m, eval: func(_, _ float64) (pvh, pvb [2][3]float64, status int) {
		calls.Add(1)

		return pvh, pvb, -1
	}}

	for range 2 {
		if _, err := s.epv00(2461000, 0.5); !errors.Is(err, ErrSofaFailure) {
			t.Fatalf("err = %v, want ErrSofaFailure", err)
		}
	}

	if got := calls.Load(); got != 2 {
		t.Errorf("%d evaluations for 2 failing requests, want 2", got)
	}

	if got := s.memo.entries(); got != 0 {
		t.Errorf("memo holds %d entries after failures only", got)
	}
}

// TestEpv00MemoConcurrentIsBitIdentical is the same claim from many goroutines
// at once, over more instants than the table holds so that they evict one
// another's. Run it under -race.
func TestEpv00MemoConcurrentIsBitIdentical(t *testing.T) {
	r := rand.New(rand.NewPCG(19, 23))

	const pool = 3000

	keys := make([][2]float64, pool)
	want := make([]epv00At, pool)

	for i := range keys {
		keys[i][0], keys[i][1] = epv00Key(r)
		want[i] = epv00Direct(t, keys[i][0], keys[i][1])
	}

	s := &sofaBase{memo: &epv00Memo{}}

	var wg sync.WaitGroup

	for g := range 8 {
		wg.Go(func() {
			rg := rand.New(rand.NewPCG(uint64(g), 29))

			for range 4000 {
				i := rg.IntN(pool)

				got, err := s.epv00(keys[i][0], keys[i][1])
				if err != nil {
					t.Error(err)

					return
				}

				if !bitsEqual(got, want[i]) {
					t.Errorf("instant %d: got %+v, want %+v", i, got, want[i])

					return
				}
			}
		})
	}

	wg.Wait()
}
