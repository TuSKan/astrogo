package plan

import (
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// failsBefore fails at every instant before until, saying why: a constraint
// whose verdict depends only on the clock, so a test of the scheduler's
// account of it does not depend on the sky.
type failsBefore struct {
	until time.Time
	why   string
}

func (c failsBefore) Check(_ Observable, t time.Time, _ *Site) (Result, error) {
	if t.Before(c.until) {
		return Result{Reason: c.why}, nil
	}

	return Result{Pass: true}, nil
}

// unscheduledFixture is a planner on the equator with no constraints of its
// own, and a two-hour window from fixedEpoch.
func unscheduledFixture(t *testing.T) (*Planner, Window) {
	t.Helper()

	loc, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	if err != nil {
		t.Fatal(err)
	}

	site, err := NewSite("equator", loc)
	if err != nil {
		t.Fatal(err)
	}

	planner, err := NewPlanner(site, nil)
	if err != nil {
		t.Fatal(err)
	}

	start := fixedEpoch()

	return planner, Window{Start: start, End: start.Add(unit.Hours(2))}
}

// scheduleOne schedules block alone with strategy, a minute's slew to it, and
// returns what is said of it unscheduled.
func scheduleOne(t *testing.T, strategy Strategy, planner *Planner, window Window, block *Block) UnscheduledBlock {
	t.Helper()

	sched, err := strategy.Schedule(planner, window, []*Block{block}, pairTransition{})
	if err != nil {
		t.Fatal(err)
	}

	if len(sched.Unscheduled) != 1 || len(sched.Blocks) != 0 && block.Cadence == nil {
		t.Fatalf("%d scheduled and %d unscheduled, want the block unscheduled", len(sched.Blocks), len(sched.Unscheduled))
	}

	return sched.Unscheduled[0]
}

// TestAnUnscheduledBlockNamesTheConstraintThatKeptItOut: every unscheduled
// block used to get one reason that could not tell one constraint from
// another (#1207). Of two constraints that each reject some placements, the
// one that rejected more is named, with its own reason. Here "too early"
// rejects every placement starting in the first 90 minutes and "never" the
// rest; moving the first's limit to 10 minutes names the second.
func TestAnUnscheduledBlockNamesTheConstraintThatKeptItOut(t *testing.T) {
	t.Parallel()

	planner, window := unscheduledFixture(t)
	star := NewStar("T", angle.Zero(), angle.Zero())

	for _, c := range []struct {
		until   time.Duration
		wantWhy string
		wantIdx int
	}{
		{90 * time.Minute, "too early", 0},
		{10 * time.Minute, "never", 1},
	} {
		constraints := []Constraint{
			failsBefore{until: window.Start.Add(time.FromGoDuration(c.until)), why: "too early"},
			failsBefore{until: window.End.Add(unit.Hours(1)), why: "never"},
		}
		block := &Block{ID: "B", Target: star, Duration: 10 * time.Minute, Constraints: constraints}

		ub := scheduleOne(t, &GreedyStrategy{}, planner, window, block)

		if ub.Cause != CauseConstraint {
			t.Errorf("limit %v: cause %v, want constraint", c.until, ub.Cause)
		}

		if ub.Constraint != constraints[c.wantIdx] {
			t.Errorf("limit %v: named %#v, want %#v", c.until, ub.Constraint, constraints[c.wantIdx])
		}

		if !strings.Contains(ub.Reason, "plan.failsBefore rejected") || !strings.HasSuffix(ub.Reason, ": "+c.wantWhy) {
			t.Errorf("limit %v: reason %q, want the constraint, its count and %q", c.until, ub.Reason, c.wantWhy)
		}

		if ub.Observed != 0 {
			t.Errorf("limit %v: observed %d, want 0", c.until, ub.Observed)
		}
	}
}

// TestABlockLongerThanItsWindowHasNoTime: a block three hours long never fits
// a two-hour window, so no placement reaches its constraints, and it is
// unscheduled for want of time, with no constraint named.
func TestABlockLongerThanItsWindowHasNoTime(t *testing.T) {
	t.Parallel()

	planner, window := unscheduledFixture(t)
	block := &Block{
		ID: "long", Target: NewStar("T", angle.Zero(), angle.Zero()), Duration: 3 * time.Hour,
		Constraints: []Constraint{failsBefore{until: window.End, why: "too early"}},
	}

	ub := scheduleOne(t, &GreedyStrategy{}, planner, window, block)

	if ub.Cause != CauseNoTime || ub.Constraint != nil {
		t.Errorf("cause %v, constraint %v; want no time and none named", ub.Cause, ub.Constraint)
	}

	if !strings.Contains(ub.Reason, "never fit") {
		t.Errorf("reason %q, want it to say the block never fit", ub.Reason)
	}
}

// TestACadenceThatRunsOutSaysHowFarItGot: a block to be observed three times,
// 90 minutes apart, is observed once in a two-hour window before the next
// observation would end past it. It is unscheduled by its cadence, having
// been observed once.
func TestACadenceThatRunsOutSaysHowFarItGot(t *testing.T) {
	t.Parallel()

	planner, window := unscheduledFixture(t)
	block := &Block{
		ID: "repeat", Target: NewStar("T", angle.Zero(), angle.Zero()), Duration: 20 * time.Minute,
		Cadence: &Cadence{MinInterval: 90 * time.Minute, Repeats: 2},
	}

	sched, err := (&GreedyStrategy{}).Schedule(planner, window, []*Block{block}, pairTransition{})
	if err != nil {
		t.Fatal(err)
	}

	if len(sched.Blocks) != 1 || len(sched.Unscheduled) != 1 {
		t.Fatalf("%d scheduled and %d unscheduled, want one observation and the block unscheduled",
			len(sched.Blocks), len(sched.Unscheduled))
	}

	ub := sched.Unscheduled[0]

	if ub.Cause != CauseCadence || ub.Observed != 1 || ub.Constraint != nil {
		t.Errorf("cause %v, observed %d, constraint %v; want cadence, once, and none named", ub.Cause, ub.Observed, ub.Constraint)
	}

	if !strings.Contains(ub.Reason, "observed 1 of 3 times") {
		t.Errorf("reason %q, want it to say 1 of 3", ub.Reason)
	}
}

// TestEveryStrategyGivesTheReason: PriorityStrategy is a greedy pass in
// priority order, and SwapOptimizedStrategy keeps its base pass's verdict for
// a block it could not place either, so each names the same constraint.
func TestEveryStrategyGivesTheReason(t *testing.T) {
	t.Parallel()

	planner, window := unscheduledFixture(t)
	never := failsBefore{until: window.End.Add(unit.Hours(1)), why: "never"}

	for _, s := range []Strategy{
		&GreedyStrategy{},
		&PriorityStrategy{},
		&SwapOptimizedStrategy{Base: &PriorityStrategy{}, MaxPasses: 3},
	} {
		block := &Block{ID: "B", Target: NewStar("T", angle.Zero(), angle.Zero()), Duration: 10 * time.Minute, Constraints: []Constraint{never}}

		ub := scheduleOne(t, s, planner, window, block)
		if ub.Cause != CauseConstraint || ub.Constraint != never || !strings.HasSuffix(ub.Reason, ": never") {
			t.Errorf("%T: cause %v, constraint %v, reason %q; want the constraint named", s, ub.Cause, ub.Constraint, ub.Reason)
		}
	}
}

func TestUnscheduledCauseString(t *testing.T) {
	t.Parallel()

	for c, want := range map[UnscheduledCause]string{
		CauseNoTime:     "no time",
		CauseConstraint: "constraint",
		CauseCadence:    "cadence",
		0:               "UnscheduledCause(0)",
	} {
		if got := c.String(); got != want {
			t.Errorf("UnscheduledCause(%d).String() = %q, want %q", int(c), got, want)
		}
	}
}
