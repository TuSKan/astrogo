package plan

import (
	"fmt"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// plannerContexts is the Context source a strategy builds for one Schedule.
func plannerContexts(planner *Planner) func(time.Time) *coord.Context {
	return newContextCache(planner.Site.Location(), planner.Site.Refraction())
}

// everyInstantContexts is the Context source the scheduler used before #481:
// a full coord.NewContext at every instant.
func everyInstantContexts(planner *Planner) func(time.Time) *coord.Context {
	return func(t time.Time) *coord.Context {
		return coord.NewContext(t, planner.Site.Location(), planner.Site.Refraction())
	}
}

// scheduleFixture is a night at Paranal with ten blocks spread in right
// ascension, of durations that are and are not whole multiples of the step,
// under an altitude constraint and a transition model that slews.
func scheduleFixture(t *testing.T) (*Planner, Window, []*Block, TransitionModel) {
	t.Helper()

	loc, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	site, err := NewSite("paranal", loc)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	planner, err := NewPlanner(site, []Constraint{Altitude{Threshold: angle.Deg(30)}})
	if err != nil {
		t.Fatalf("NewPlanner: %v", err)
	}

	start := time.Date(2026, time.March, 20, 0, 0, 0, 0, time.LocationUTC)
	window := Window{Start: start, End: start.Add(unit.Hours(9))}

	blocks := make([]*Block, 0, 10)
	for i := range 10 {
		blocks = append(blocks, &Block{
			ID:       fmt.Sprintf("B%d", i),
			Target:   NewStar(fmt.Sprintf("T%d", i), angle.Hour(6+1.3*float64(i)), angle.Deg(-60+11*float64(i))),
			Duration: time.Duration(10+7*i) * time.Minute,
			Priority: float64(10 - i),
		})
	}

	return planner, window, blocks, &BasicTransitionModel{BaseSetup: time.Minute, SlewRate: 2}
}

// TestScheduleThroughTheContextCache holds the greedy and swap-optimized
// strategies, which evaluate through one Context cache per Schedule since
// #481, to the schedules they build on a full Context at every instant.
//
// Which block goes where is a sequence of discrete decisions at whole steps,
// so the order must agree exactly: a decision changes only if a constraint
// sits within the cache's ≲0.1″ of its threshold at some step, and none does
// here. The instants agree to a bound rather than exactly, because since #485
// the slew between blocks goes through the cache too, and 0.1″ at the 2°/s
// used here is ~14 µs of slew, carried forward block to block; the bound is
// 0.01 s. Scores differ by that 0.1″ of altitude, about 1e-5 at these
// priorities; the bound is 1e-3.
func TestScheduleThroughTheContextCache(t *testing.T) {
	t.Parallel()

	planner, window, blocks, transition := scheduleFixture(t)

	for _, run := range []struct {
		name     string
		schedule func(ctxAt func(time.Time) *coord.Context) (*Schedule, error)
	}{
		{"greedy", func(ctxAt func(time.Time) *coord.Context) (*Schedule, error) {
			return (&GreedyStrategy{}).schedule(planner, window, blocks, transition, ctxAt)
		}},
		{"swap", func(ctxAt func(time.Time) *coord.Context) (*Schedule, error) {
			return (&SwapOptimizedStrategy{Base: &PriorityStrategy{}, MaxPasses: 3}).schedule(planner, window, blocks, transition, ctxAt)
		}},
	} {
		got, err := run.schedule(plannerContexts(planner))
		if err != nil {
			t.Fatalf("%s: %v", run.name, err)
		}

		want, err := run.schedule(everyInstantContexts(planner))
		if err != nil {
			t.Fatalf("%s: reference: %v", run.name, err)
		}

		if len(got.Blocks) != len(want.Blocks) || len(got.Blocks) < 3 {
			t.Fatalf("%s: %d blocks placed, a full Context per instant places %d (and the fixture wants at least 3)",
				run.name, len(got.Blocks), len(want.Blocks))
		}

		for i := range got.Blocks {
			g, w := got.Blocks[i], want.Blocks[i]

			if g.Block.ID != w.Block.ID ||
				math.Abs(g.Window.Start.Sub(w.Window.Start).Seconds()) > 0.01 ||
				math.Abs(g.Window.End.Sub(w.Window.End).Seconds()) > 0.01 {
				t.Errorf("%s: block %d is %s at %v, a full Context per instant places %s at %v",
					run.name, i, g.Block.ID, g.Window.Start, w.Block.ID, w.Window.Start)
			}

			if d := math.Abs(g.Score - w.Score); d > 1e-3 {
				t.Errorf("%s: %s scored %.6f, a full Context per instant %.6f", run.name, g.Block.ID, g.Score, w.Score)
			}
		}
	}
}

// TestBlockIsScoredAtItsMidpoint is #481's scoring defect. A block is scored
// at its midpoint, and was handed the Context of the constraint step nearest
// it, whose Earth is turned to that step's instant. With a 10-minute step and
// a 15-minute block the nearest step is 2.5 minutes away: the score read the
// altitude 2.5 minutes late.
//
// The reference is the public Scorer at the midpoint, which builds its own
// full Context there.
func TestBlockIsScoredAtItsMidpoint(t *testing.T) {
	t.Parallel()

	planner, window, _, _ := scheduleFixture(t)

	// Rising in the east at the window's start, where altitude changes
	// fastest, so a Context at the wrong instant moves the score most.
	block := &Block{
		ID:       "rising",
		Target:   NewStar("rising", angle.Hour(8.5), angle.Deg(-20)),
		Duration: 15 * time.Minute,
		Priority: 1,
	}

	sched, err := (&GreedyStrategy{Step: 10 * time.Minute}).Schedule(planner, window, []*Block{block}, &BasicTransitionModel{BaseSetup: time.Minute})
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	if len(sched.Blocks) != 1 {
		t.Fatalf("%d blocks placed, want 1", len(sched.Blocks))
	}

	placed := sched.Blocks[0]
	mid := placed.Window.Start.Add(placed.Window.End.Sub(placed.Window.Start) / 2)

	want, err := Scorer{Site: planner.Site, Constraints: planner.Constraints}.Score(block.Target, mid)
	if err != nil {
		t.Fatalf("Score: %v", err)
	}

	if d := math.Abs(placed.Score - want); d > 1e-3 {
		t.Errorf("the block at %v scored %.4f; scored at its midpoint %v it is %.4f", placed.Window.Start, placed.Score, mid, want)
	}
}

// TestSetTimeProbesThroughAtTime holds estimateHoursUntilSet, whose probes are
// derived from the scoring Context with AtTime since #481, to the same probes
// on full Contexts. The furthest probe is eight hours out, where AtTime is
// ≲0.8″ off; the estimate is a linear interpolation between probes, so that
// moves it by milliseconds. The bound is 1e-3 h, 3.6 s.
func TestSetTimeProbesThroughAtTime(t *testing.T) {
	t.Parallel()

	planner, window, blocks, _ := scheduleFixture(t)

	reference := func(obj Observable, t0 time.Time, currentAlt float64) float64 {
		if currentAlt <= 0 {
			return 0
		}

		for _, offset := range [5]time.Duration{30 * time.Minute, time.Hour, 2 * time.Hour, 4 * time.Hour, 8 * time.Hour} {
			ft := t0.Add(time.FromGoDuration(offset))

			pos, err := obj.Position(ft)
			if err != nil {
				t.Fatalf("reference probe: Position: %v", err)
			}

			aa, err := observedAltAz(obj, ft, everyInstantContexts(planner)(ft), pos)
			if err != nil {
				t.Fatalf("reference probe: observedAltAz: %v", err)
			}

			if aa.Alt().Degrees() <= 0 {
				return offset.Hours() * (currentAlt / (currentAlt - aa.Alt().Degrees()))
			}
		}

		return math.Inf(1)
	}

	compared := 0

	for _, b := range blocks {
		for at := window.Start; at.Before(window.End); at = at.Add(unit.Hours(1.5)) {
			ctx := coord.NewContext(at, planner.Site.Location(), planner.Site.Refraction())

			pos, err := b.Target.Position(at)
			if err != nil {
				t.Fatalf("Position: %v", err)
			}

			aa, err := observedAltAz(b.Target, at, ctx, pos)
			if err != nil {
				t.Fatalf("observedAltAz: %v", err)
			}

			got := estimateHoursUntilSet(b.Target, at, ctx, aa.Alt().Degrees())
			want := reference(b.Target, at, aa.Alt().Degrees())

			if math.IsInf(want, 1) || want == 0 {
				if got != want {
					t.Errorf("%s at %v: %v hours until set, full Contexts say %v", b.ID, at, got, want)
				}

				continue
			}

			compared++

			if d := math.Abs(got - want); d > 1e-3 {
				t.Errorf("%s at %v: %.6f hours until set, full Contexts say %.6f", b.ID, at, got, want)
			}
		}
	}

	if compared < 10 {
		t.Fatalf("only %d estimates were interpolations; the fixture no longer exercises the probes", compared)
	}
}
