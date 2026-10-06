package plan

import (
	"errors"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

var errOverheadUnknown = errors.New("overhead_errors_test: overhead could not be computed")

// failingTransition is a TransitionModel that cannot answer for one call, or
// for every call: the shape of a slew model whose target position lookup
// failed. Every other call costs nothing.
type failingTransition struct {
	failAt int // the call, counted from 1, that fails; 0 fails every call
	calls  int
}

func (m *failingTransition) Overhead(_ TransitionContext) (time.Duration, error) {
	m.calls++
	if m.failAt == 0 || m.calls == m.failAt {
		return 0, errOverheadUnknown
	}

	return 0, nil
}

// overheadFixture is a site, a planner with no constraints of its own, a two
// hour window and two ten-minute blocks on stars at the celestial equator.
func overheadFixture(t *testing.T) (planner *Planner, window Window, b1, b2 *Block) {
	t.Helper()

	planner, err := NewPlanner(predicateSite(t), nil)
	if err != nil {
		t.Fatalf("NewPlanner: %v", err)
	}

	start := fixedEpoch()
	window = Window{Start: start, End: start.Add(unit.Hours(2))}
	b1 = &Block{ID: "B1", Target: NewStar("A", angle.Zero(), angle.Zero()), Duration: 10 * time.Minute}
	b2 = &Block{ID: "B2", Target: NewStar("B", angle.Zero(), angle.Zero()), Duration: 10 * time.Minute}

	return planner, window, b1, b2
}

// TestSchedulerReportsAnOverheadThatCannotBeComputed is #454's scheduler half.
//
// Every strategy skipped a candidate whose transition overhead failed, so a
// block whose target could not be positioned for the slew never placed and
// nothing said why. The greedy pass also refined its estimate with a second
// call whose error it discarded, which placed the block with no setup time at
// all. A constraint that cannot be evaluated for the same target already
// fails the schedule, and this now does too.
func TestSchedulerReportsAnOverheadThatCannotBeComputed(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		strategy Strategy
		failAt   int
	}{
		{"greedy, first estimate", &GreedyStrategy{}, 1},
		{"greedy, refined estimate", &GreedyStrategy{}, 2},
		{"priority", &PriorityStrategy{}, 1},
		{"swap-optimized", &SwapOptimizedStrategy{}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			planner, window, b1, _ := overheadFixture(t)
			scheduler := NewScheduler(planner, tc.strategy, &failingTransition{failAt: tc.failAt})

			sched, err := scheduler.BuildSchedule(window, []*Block{b1})
			if !errors.Is(err, errOverheadUnknown) {
				t.Fatalf("BuildSchedule returned %v (schedule %v), want it to wrap the overhead's error", err, sched)
			}

			if !strings.Contains(err.Error(), "transition from the start to B1") {
				t.Errorf("BuildSchedule: %v, want it to name the transition", err)
			}
		})
	}
}

// TestSwapAndInsertPassesReportAnOverheadFailure reaches the two passes with
// a schedule built by hand, as TestSwapAndInsertPassesReportAConstraintFailure
// does: through Schedule, the greedy seed meets the failing model first.
func TestSwapAndInsertPassesReportAnOverheadFailure(t *testing.T) {
	t.Parallel()

	planner, window, b1, b2 := overheadFixture(t)
	start := window.Start
	strategy := &SwapOptimizedStrategy{}

	placed := func(b *Block, offset time.Duration) ScheduledBlock {
		return ScheduledBlock{
			Block: b,
			Window: Window{
				Start: start.Add(time.FromGoDuration(offset)),
				End:   start.Add(time.FromGoDuration(offset) + unit.Minutes(10)),
			},
		}
	}

	// Two adjacent blocks, so the swap is considered. It asks for two
	// transitions, in order: into B2 from the start of the window, where B2
	// moves to, and from B2 to B1, which follows it. Each failure must be
	// reported as its own.
	for _, tc := range []struct {
		failAt int
		names  string
	}{
		{1, "transition from the start to B2"},
		{2, "transition from B2 to B1"},
	} {
		swap := &Schedule{Window: window, Blocks: []ScheduledBlock{placed(b1, 0), placed(b2, 10*time.Minute)}}

		_, err := strategy.swapPass(swap, planner, &failingTransition{failAt: tc.failAt}, time.Minute,
			plannerContexts(planner), newTabuList(2), 0)
		if !errors.Is(err, errOverheadUnknown) || !strings.Contains(err.Error(), tc.names) {
			t.Errorf("swapPass, call %d failing: %v, want it to wrap the overhead's error and name the %s",
				tc.failAt, err, tc.names)
		}
	}

	// One placed block and one waiting for a gap. The first gap tried is the
	// one before B1: into B2 from the start of the window, then from B2 to
	// B1, which the gap ends at.
	for _, tc := range []struct {
		failAt int
		names  string
	}{
		{1, "transition from the start to B2"},
		{2, "transition from B2 to B1"},
	} {
		insert := &Schedule{
			Window:      window,
			Blocks:      []ScheduledBlock{placed(b1, time.Hour)},
			Unscheduled: []UnscheduledBlock{{Block: b2}},
		}

		_, err := strategy.insertPass(insert, planner, window, &failingTransition{failAt: tc.failAt}, time.Minute,
			plannerContexts(planner))
		if !errors.Is(err, errOverheadUnknown) || !strings.Contains(err.Error(), tc.names) {
			t.Errorf("insertPass, call %d failing: %v, want it to wrap the overhead's error and name the %s",
				tc.failAt, err, tc.names)
		}
	}
}
