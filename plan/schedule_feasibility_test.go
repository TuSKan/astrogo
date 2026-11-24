package plan

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/unit"
)

// pairTransition costs half an hour between two named blocks, either way, and
// a minute for any other move: the shape of a slew between targets on
// opposite sides of the sky, without the astronomy that would make a test of
// it depend on the epoch.
type pairTransition struct{ a, b string }

func (m pairTransition) Overhead(tr Transition) (unit.Duration, error) {
	if tr.FromBlock != nil {
		from, to := tr.FromBlock.ID, tr.ToBlock.ID
		if (from == m.a && to == m.b) || (from == m.b && to == m.a) {
			return unit.Minutes(30), nil
		}
	}

	return unit.Minutes(1), nil
}

// assertExecutable checks the property a schedule exists to have: every block
// starts no earlier than the transition model says the telescope can reach
// it, and its SetupTime is that transition rather than one it no longer has.
func assertExecutable(t *testing.T, sched *Schedule, model TransitionModel) {
	t.Helper()

	prevEnd := sched.Window.Start

	var prev *Block

	for k, sb := range sched.Blocks {
		need, err := model.Overhead(Transition{FromBlock: prev, ToBlock: sb.Block})
		if err != nil {
			t.Fatalf("overhead into %s: %v", sb.Block.ID, err)
		}

		// A millisecond for the float seconds a time.Time is built from.
		if got := sb.Window.Start.Sub(prevEnd).Seconds(); got < need.Seconds()-1e-3 {
			t.Errorf("[%d] %s starts %.0f s after its predecessor ends, but the slew to it takes %.0f s",
				k, sb.Block.ID, got, need.Seconds())
		}

		if sb.SetupTime != need {
			t.Errorf("[%d] %s reports SetupTime %v, but the transition into it is %v", k, sb.Block.ID, sb.SetupTime, need)
		}

		prev, prevEnd = sb.Block, sb.Window.End
	}
}

// TestSwapOptimizedLeavesTimeForEverySlew is #547's reproduction. The base
// schedule X, A, B is executable. Swapping X and A is a plateau move — the
// same target, so the same scores — and it puts X directly before B, a
// half-hour slew. The only check was that X ended before B started, so the
// swap was taken; the swap that would have moved X past B ends outside the
// window and was refused; and B was left starting a minute after X with a
// one-minute SetupTime it no longer had.
func TestSwapOptimizedLeavesTimeForEverySlew(t *testing.T) {
	t.Parallel()

	loc, err := coord.NewGeodetic(angle.Latitude{}, angle.Longitude{}, 0)
	if err != nil {
		t.Fatal(err)
	}

	site, err := NewSite("equator", loc.Lat(), loc.Lon(), loc.Height())
	if err != nil {
		t.Fatal(err)
	}

	planner, err := NewPlanner(site, nil)
	if err != nil {
		t.Fatal(err)
	}

	start := fixedEpoch()
	window := Window{Start: start, End: start.Add(unit.Minutes(65))}
	star := NewStar("T", angle.Zero(), angle.Zero())
	block := func(id string, priority float64) *Block {
		return &Block{ID: id, Target: star, Duration: unit.Minutes(20), Priority: priority}
	}

	model := pairTransition{"X", "B"}

	sched, err := (&SwapOptimizedStrategy{Base: &PriorityStrategy{}, MaxPasses: 5}).
		Schedule(planner, window, []*Block{block("X", 3), block("A", 2), block("B", 2)}, model)
	if err != nil {
		t.Fatal(err)
	}

	assertExecutable(t, sched, model)
}

// TestInsertPassLeavesTimeForTheNextSlew covers the other move. A block that
// fits a gap after the slew into it must still leave time for the slew out of
// it, to the block the gap ends at: here U fits the ten minutes between P and
// N, but N is half an hour from U. Until #547 it was inserted there anyway.
func TestInsertPassLeavesTimeForTheNextSlew(t *testing.T) {
	t.Parallel()

	planner, window, _, _ := overheadFixture(t)
	start := window.Start
	star := NewStar("T", angle.Zero(), angle.Zero())

	at := func(id string, from, to unit.Duration) ScheduledBlock {
		return ScheduledBlock{
			Block: &Block{ID: id, Target: star, Duration: to - from},
			Window: Window{
				Start: start.Add(from),
				End:   start.Add(to),
			},
			SetupTime: unit.Minutes(1),
		}
	}

	model := pairTransition{"U", "N"}
	sched := &Schedule{
		Window: window,
		Blocks: []ScheduledBlock{at("P", unit.Minutes(1), unit.Minutes(21)), at("N", unit.Minutes(31), unit.Minutes(51))},
		Unscheduled: []UnscheduledBlock{{
			Block: &Block{ID: "U", Target: star, Duration: unit.Minutes(5)},
		}},
	}

	inserted, err := (&SwapOptimizedStrategy{}).insertPass(sched, planner, window, model, unit.Minutes(1), plannerContexts(t, planner, window.Start), nil)
	if err != nil {
		t.Fatal(err)
	}

	if !inserted || len(sched.Unscheduled) != 0 {
		t.Fatalf("U was not inserted anywhere (inserted %v, %d unscheduled); there is room after N", inserted, len(sched.Unscheduled))
	}

	assertExecutable(t, sched, model)
}
