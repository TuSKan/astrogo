package plan

import (
	"fmt"
	"math"
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// RankObservables evaluates every target at one instant from one site, so the
// Context that holds that instant's rotation and observer is the same for all of
// them. It used to be built once per target, about 100 µs each (#916). These
// tests hold the two things that change: the Context is shared, and no score
// moves.

func rcEpoch() time.Time {
	return time.Date(2026, time.March, 20, 22, 0, 0, 0, time.LocationUTC)
}

func rcSite(tb testing.TB) *Site {
	tb.Helper()

	loc, err := coord.NewGeodetic(angle.Deg(2.17), angle.Deg(41.39), 0)
	if err != nil {
		tb.Fatal(err)
	}

	site, err := NewSite("Barcelona", loc)
	if err != nil {
		tb.Fatal(err)
	}

	return site
}

// rcObjects is n stars spread over the sky, then the Sun, the Moon and four
// planets: a list a planner ranks, with both kinds of target in it.
func rcObjects(n int) []Observable {
	objs := make([]Observable, 0, n+6)

	for i := range n {
		ra := math.Mod(float64(i)*137.50776405, 360)
		dec := -70 + 140*math.Mod(float64(i)*0.6180339887, 1)
		objs = append(objs, NewStar(fmt.Sprintf("S%d", i), angle.Deg(ra), angle.Deg(dec)))
	}

	prov := eph.Default()

	return append(objs,
		NewSun(prov), NewMoon(prov), NewVenus(prov), NewMars(prov), NewJupiter(prov), NewSaturn(prov))
}

// contextRecorder passes every target and records which Context it was handed.
type contextRecorder struct {
	mu   sync.Mutex
	seen map[*coord.Context]struct{}
}

func (r *contextRecorder) Check(obj Observable, t time.Time, site *Site) (Result, error) {
	return r.CheckCtx(obj, t, site, coord.NewContext(t, site.Location(), site.Refraction()))
}

func (r *contextRecorder) CheckCtx(_ Observable, _ time.Time, _ *Site, ctx *coord.Context) (Result, error) {
	r.mu.Lock()
	r.seen[ctx] = struct{}{}
	r.mu.Unlock()

	return Result{Pass: true}, nil
}

// TestRankObservablesBuildsOneContext: every constraint check of one ranking is
// handed the same Context. A Context per target is 100 µs and 1.25 KB each, and
// the one the first target needs is the one all the others need.
func TestRankObservablesBuildsOneContext(t *testing.T) {
	rec := &contextRecorder{seen: map[*coord.Context]struct{}{}}

	ranked, err := RankObservables(rcObjects(200), rcEpoch(), rcSite(t), rec)
	if err != nil {
		t.Fatal(err)
	}

	if len(ranked) == 0 {
		t.Fatal("nothing ranked: the recorder passes every target, so the list was not scored")
	}

	if got := len(rec.seen); got != 1 {
		t.Errorf("%d different Contexts were handed to the constraint for %d targets, want 1", got, len(ranked))
	}
}

// TestRankObservablesMatchesScoringEachTarget is the claim the change may make:
// the scores are the ones a Scorer gives each target when it builds a Context
// for it, to the last bit. It runs the five built-in constraints, which read the
// shared Context from every worker at once, so under -race it is also the check
// that none of them writes to it.
func TestRankObservablesMatchesScoringEachTarget(t *testing.T) {
	site := rcSite(t)
	when := rcEpoch()
	objs := rcObjects(150)
	constraints := []Constraint{
		Altitude{Threshold: angle.Deg(10)},
		Airmass{Threshold: 3},
		Sun{Threshold: angle.Deg(-6)},
		MoonSep{Threshold: angle.Deg(5)},
		MoonIllum{Threshold: 0.99},
	}

	// What RankObservables did before: a Scorer per target, each with no Context
	// and so each building its own.
	want := map[string]float64{}

	for _, obj := range objs {
		score, err := Scorer{Site: site, Constraints: constraints}.Score(obj, when)
		if err != nil {
			t.Fatalf("%s: %v", obj.Name(), err)
		}

		if score > 0 {
			want[obj.Name()] = score
		}
	}

	if len(want) < 20 {
		t.Fatalf("only %d targets score above zero: the fixture no longer exercises the ranking", len(want))
	}

	if _, up := want["Jupiter"]; !up {
		t.Fatal("Jupiter does not score: the fixture no longer has a moving body in the ranking")
	}

	got, err := RankObservables(objs, when, site, constraints...)
	if err != nil {
		t.Fatal(err)
	}

	if len(got) != len(want) {
		t.Fatalf("ranked %d targets, want %d", len(got), len(want))
	}

	for i, g := range got {
		w, ok := want[g.Object.Name()]
		if !ok {
			t.Errorf("%s is ranked but scores zero on its own", g.Object.Name())

			continue
		}

		if math.Float64bits(g.Score) != math.Float64bits(w) {
			t.Errorf("%s: score %v, want %v", g.Object.Name(), g.Score, w)
		}

		if i > 0 && got[i-1].Score < g.Score {
			t.Errorf("rank %d (%v) outscores rank %d (%v)", i, g.Score, i-1, got[i-1].Score)
		}
	}
}

// TestRankObservablesEmptyListBuildsNothing: nothing to score is not an
// occasion to build a Context, or to need a site.
func TestRankObservablesEmptyListBuildsNothing(t *testing.T) {
	ranked, err := RankObservables(nil, rcEpoch(), nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(ranked) != 0 {
		t.Errorf("ranked %d targets from an empty list", len(ranked))
	}
}
