package plan

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TransitionContext was comparable before it carried a Context source, so a
// caller may compare two with ==. The source is a pointer so that it stays
// comparable; a func field would not, and this would not compile.
func TestTransitionContextStaysComparable(t *testing.T) {
	t.Parallel()

	plain := TransitionContext{}
	cached := TransitionContext{contexts: &contextSource{}}

	if plain == cached {
		t.Error("a TransitionContext with a Context source compares equal to one without")
	}
}

// A target Overhead can place but cannot turn into alt/az fails the slew from
// either end, rather than slewing from a zero position.
func TestOverheadReportsAnEndItCannotObserve(t *testing.T) {
	t.Parallel()

	loc, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	site, err := NewSite("s", loc)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	star := &Block{ID: "star", Target: NewStar("a", angle.Hour(5.5), angle.Deg(-5))}
	moving := &Block{ID: "moving", Target: errMovingBody{}}
	model := &BasicTransitionModel{SlewRate: 2}
	start := time.Date(2026, time.March, 20, 1, 0, 0, 0, time.LocationUTC)

	for _, c := range []struct {
		name     string
		from, to *Block
	}{
		{"from AltAz", moving, star},
		{"to AltAz", star, moving},
	} {
		_, err := model.Overhead(TransitionContext{FromBlock: c.from, ToBlock: c.to, FromTime: start, ToTime: start, Site: site})
		if !errors.Is(err, errMovingBodyFails) || !strings.Contains(err.Error(), c.name) {
			t.Errorf("%s: Overhead returned %v, want the %s failure", c.name, err, c.name)
		}
	}
}

// TestOverheadThroughTheContextCache is #485: a TransitionContext carrying the
// strategy's Context cache gives the slew the full-Context answer, at AtTime's
// ≲0.1″, which at the 2°/s used here is 14 µs of slew. Both the shared-instant
// path and the two-instant one are covered.
func TestOverheadThroughTheContextCache(t *testing.T) {
	t.Parallel()

	loc, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	site, err := NewSite("s", loc)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	from := &Block{ID: "from", Target: NewStar("a", angle.Hour(5.5), angle.Deg(-5))}
	to := &Block{ID: "to", Target: NewStar("b", angle.Hour(8.1), angle.Deg(-40))}
	model := &BasicTransitionModel{SlewRate: 2}
	start := time.Date(2026, time.March, 20, 1, 0, 0, 0, time.LocationUTC)
	cache := newContextCache(site.Location(), site.Refraction())

	for _, toTime := range []time.Time{start, start.Add(unit.Minutes(3))} {
		tc := TransitionContext{FromBlock: from, ToBlock: to, FromTime: start, ToTime: toTime, Site: site}

		full, err := model.Overhead(tc)
		if err != nil {
			t.Fatalf("Overhead with full Contexts: %v", err)
		}

		asked := 0
		tc.contexts = &contextSource{at: func(at time.Time) *coord.Context {
			asked++

			return cache(at)
		}}

		cached, err := model.Overhead(tc)
		if err != nil {
			t.Fatalf("Overhead through the cache: %v", err)
		}

		// The field is honored, not merely harmless: one Context per instant.
		if want := 1 + btoi(!toTime.Equal(start)); asked != want {
			t.Errorf("ToTime %v: ContextAt asked %d times, want %d", toTime, asked, want)
		}

		if full <= 0 {
			t.Fatalf("no slew between two targets 40° apart: %v", full)
		}

		if d := math.Abs((cached - full).Seconds()); d > 1e-3 {
			t.Errorf("ToTime %v: slew %v through the cache, %v with full Contexts", toTime, cached, full)
		}
	}
}

func btoi(b bool) int {
	if b {
		return 1
	}

	return 0
}

// BenchmarkGreedyStrategy_10Slew is BenchmarkGreedyStrategy_10 with the
// transition model slewing, so BasicTransitionModel.Overhead's Contexts are
// in the measurement: on main before #485 they were ~86% of it.
func BenchmarkGreedyStrategy_10Slew(b *testing.B) {
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := NewSite("Bench", loc)
	planner, _ := NewPlanner(site, nil)
	tm := &BasicTransitionModel{BaseSetup: 0, SlewRate: 2}
	blocks := makeBlocks(10)
	start := time.Date(2026, time.March, 20, 0, 0, 0, 0, time.LocationUTC)
	window := Window{Start: start, End: start.Add(unit.Minutes(150))}

	for b.Loop() {
		_, _ = (&GreedyStrategy{}).Schedule(planner, window, blocks, tm)
	}
}
