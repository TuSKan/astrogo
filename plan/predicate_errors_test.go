package plan

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/logging"
	"github.com/TuSKan/astrogo/time"
)

var errConstraintUnavailable = errors.New("predicate_errors_test: constraint could not be evaluated")

// failingConstraint cannot answer — the shape of an ephemeris lookup that
// failed or a provider that could not be reached.
type failingConstraint struct{}

func (failingConstraint) Check(_ Observable, _ time.Time, _ *Site) (Result, error) {
	return Result{}, errConstraintUnavailable
}

var errPositionUnavailable = errors.New("predicate_errors_test: position could not be computed")

// unreachableTarget is an Observable whose position lookup always fails — a
// target whose ephemeris provider is down.
//
// evaluateCandidate builds its own constraint (Altitude) internally rather
// than taking the caller's, so failingConstraint cannot reach it; failing at
// the Observable is the way in, and is the more realistic failure anyway.
type unreachableTarget struct{}

func (unreachableTarget) Name() string { return "Unreachable" }

func (unreachableTarget) Position(_ time.Time) (coord.ICRS, error) {
	return coord.ICRS{}, errPositionUnavailable
}

func (unreachableTarget) GetDetails(_ *coord.Context, _ DetailOverrides) (*TargetDetails, error) {
	return nil, errPositionUnavailable
}

// predicateSite builds the site these tests share.
func predicateSite(t *testing.T) *Site {
	t.Helper()

	loc, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	site, err := NewSite("predicates", loc)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	return site
}

// TestSchedulerReportsAConstraintThatCannotBeEvaluated is the fix for #177.
//
// # What was wrong
//
// checkConstraintsIntervalCtx read `if err != nil || !res.Pass` and returned a
// bare false. So a constraint that could not be evaluated was indistinguishable
// from one the target genuinely failed: the block was dropped, and the caller
// received a schedule that looked complete.
//
// Constraint.Check returns an error precisely because a check can fail rather
// than merely be false. Discarding that at the last step is the shape of #102,
// where a swallowed error turned a CDS outage into "target not found".
//
// A schedule silently missing a block is the worst version of this, because
// nothing about the result invites suspicion.
func TestSchedulerReportsAConstraintThatCannotBeEvaluated(t *testing.T) {
	t.Parallel()

	site := predicateSite(t)

	planner, err := NewPlanner(site, []Constraint{failingConstraint{}})
	if err != nil {
		t.Fatalf("NewPlanner: %v", err)
	}

	start := fixedEpoch()
	window := Window{Start: start, End: start.Add(1 * time.Hour)}
	block := &Block{ID: "B1", Target: NewStar("T", angle.Zero(), angle.Zero()), Duration: 10 * time.Minute}

	for _, tc := range []struct {
		name     string
		strategy Strategy
	}{
		{"greedy", &GreedyStrategy{}},
		{"swap-optimized", &SwapOptimizedStrategy{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			scheduler := NewScheduler(planner, tc.strategy, &BasicTransitionModel{BaseSetup: 0})

			sched, err := scheduler.BuildSchedule(window, []*Block{block})
			if !errors.Is(err, errConstraintUnavailable) {
				t.Fatalf("BuildSchedule returned %v (schedule %v), want it to wrap the "+
					"constraint's error.\n  A constraint that cannot be evaluated is not "+
					"a constraint the target failed; dropping the block leaves the "+
					"caller a schedule that looks complete.", err, sched)
			}
		})
	}
}

// TestFindReportsAConstraintThatCannotBeEvaluated covers the other bisection
// predicate, in the visibility solver rather than the scheduler.
//
// Find's own signature already returned an error; only the closure driving the
// interval search discarded one.
func TestFindReportsAConstraintThatCannotBeEvaluated(t *testing.T) {
	t.Parallel()

	site := predicateSite(t)
	start := fixedEpoch()

	_, err := Find(
		mockObject{pos: coord.NewICRS(angle.Zero(), angle.Zero())},
		site,
		[]Constraint{failingConstraint{}},
		start, start.Add(2*time.Hour), 10*time.Minute,
	)

	if !errors.Is(err, errConstraintUnavailable) {
		t.Errorf("Find returned %v, want it to wrap the constraint's error", err)
	}
}

// TestObservableWindowsReportsAFailureDuringRefinement covers the subtler half.
//
// ObservableWindows already propagated the error from its sampling loop. The
// closure handed to refineBisect did not — so a check that failed *during*
// bisection did not drop the window, it moved the refined boundary, and the
// caller got a rise or set time that was quietly wrong rather than absent.
func TestObservableWindowsReportsAFailureDuringRefinement(t *testing.T) {
	t.Parallel()

	site := predicateSite(t)
	start := fixedEpoch()

	_, err := ObservableWindows(
		NewStar("T", angle.Zero(), angle.Zero()),
		start, start.Add(2*time.Hour), 10*time.Minute,
		site,
		failingConstraint{},
	)

	if !errors.Is(err, errConstraintUnavailable) {
		t.Errorf("ObservableWindows returned %v, want it to wrap the constraint's error", err)
	}
}

// TestVisibleTonightReportsWhyACandidateWasSkipped covers the one place that
// deliberately keeps skipping.
//
// VisibleTonight's contract is to skip a candidate it cannot evaluate rather
// than fail the whole query — one unreachable kernel should not cost the caller
// the other forty. But every skip returned the same bare false as "too faint"
// or "never rises", so a night's list could come back short because a provider
// was down and nothing said which it was.
//
// The skip stays. What changed is that the reason now leaves the function: a
// Warn line for a human, and an error a program can test with errors.Is. A log
// line is not a signal a caller can act on.
func TestVisibleTonightReportsWhyACandidateWasSkipped(t *testing.T) {
	// Not parallel: it installs the process-wide logger.
	defer logging.Set(nil)

	var buf bytes.Buffer

	logging.Set(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))

	site := predicateSite(t)
	start := fixedEpoch()

	// A candidate whose position cannot be computed — the shape of a target
	// backed by an ephemeris provider that could not be reached. The step is
	// a legal one (ObservableWindows rejects anything over 15m outright), so
	// the only thing that can fail here is the lookup itself.
	cfg := visibleTonightConfig{step: 10 * time.Minute, minAltitude: angle.Deg(10)}

	_, ok, why := evaluateCandidate(
		t.Context(),
		visibleCandidate{obj: unreachableTarget{}},
		start, start.Add(2*time.Hour), site, nil, 6.0, cfg,
	)
	if ok {
		t.Fatal("precondition: this candidate should not evaluate cleanly")
	}

	if !errors.Is(why, errPositionUnavailable) {
		t.Errorf("evaluateCandidate returned reason %v, want it to wrap the lookup's error.\n"+
			"  Without it the caller cannot tell an unreachable provider from a target that is simply not up.", why)
	}

	out := buf.String()
	if !strings.Contains(out, "candidate skipped") {
		t.Errorf("a skipped candidate produced no warning:\n%s\n"+
			"  The skip is deliberate, but a silent one leaves the caller a "+
			"short list with no way to tell why.", out)
	}
}

// TestSwapAndInsertPassesReportAConstraintFailure reaches the two passes
// directly.
//
// Going through Schedule cannot: SwapOptimizedStrategy seeds with a greedy
// pass, so a constraint that errors kills the seed before a swap is ever
// attempted, and the forwarding branches inside swapPass and insertPass stay
// unexecuted. Calling them with a schedule built by hand is the only way to
// exercise the paths this change added, and they are unexported, so an
// in-package test can.
func TestSwapAndInsertPassesReportAConstraintFailure(t *testing.T) {
	t.Parallel()

	site := predicateSite(t)

	planner, err := NewPlanner(site, []Constraint{failingConstraint{}})
	if err != nil {
		t.Fatalf("NewPlanner: %v", err)
	}

	start := fixedEpoch()
	window := Window{Start: start, End: start.Add(2 * time.Hour)}

	b1 := &Block{ID: "B1", Target: NewStar("A", angle.Zero(), angle.Zero()), Duration: 10 * time.Minute}
	b2 := &Block{ID: "B2", Target: NewStar("B", angle.Zero(), angle.Zero()), Duration: 10 * time.Minute}

	strategy := &SwapOptimizedStrategy{}
	transition := &BasicTransitionModel{BaseSetup: 0}

	// Two adjacent scheduled blocks, so a swap is actually considered, plus one
	// unscheduled so the insert pass has something to place.
	sched := &Schedule{
		Window: window,
		Blocks: []ScheduledBlock{
			{Block: b1, Window: Window{Start: start, End: start.Add(10 * time.Minute)}},
			{Block: b2, Window: Window{Start: start.Add(10 * time.Minute), End: start.Add(20 * time.Minute)}},
		},
		Unscheduled: []UnscheduledBlock{{Block: b1}},
	}

	if _, err := strategy.swapPass(sched, planner, transition, time.Minute, newTabuList(2), 0); !errors.Is(err, errConstraintUnavailable) {
		t.Errorf("swapPass returned %v, want it to wrap the constraint's error", err)
	}

	if _, err := strategy.insertPass(sched, planner, window, transition, time.Minute); !errors.Is(err, errConstraintUnavailable) {
		t.Errorf("insertPass returned %v, want it to wrap the constraint's error", err)
	}
}

// seedStrategy returns a canned schedule without evaluating any constraint,
// standing in for SwapOptimizedStrategy's greedy seed.
//
// It exists because the seed and the passes share one constraint set: an
// always-failing constraint kills the greedy seed before a swap is attempted,
// so the error forwarding inside SwapOptimizedStrategy.Schedule is
// unreachable through the public entry point. Replacing the seed reaches it.
type seedStrategy struct{ sched *Schedule }

func (s seedStrategy) Schedule(_ *Planner, _ Window, _ []*Block, _ TransitionModel) (*Schedule, error) {
	return s.sched, nil
}

// TestSwapOptimizedScheduleForwardsAPassFailure covers the two error forwards
// in SwapOptimizedStrategy.Schedule.
//
// Two cases, because the passes run in order and the first to fail hides the
// second: with two adjacent blocks a swap is considered and swapPass fails;
// with one block no swap is possible, swapPass returns cleanly, and insertPass
// fails on the unscheduled block instead.
func TestSwapOptimizedScheduleForwardsAPassFailure(t *testing.T) {
	t.Parallel()

	site := predicateSite(t)

	planner, err := NewPlanner(site, []Constraint{failingConstraint{}})
	if err != nil {
		t.Fatalf("NewPlanner: %v", err)
	}

	start := fixedEpoch()
	window := Window{Start: start, End: start.Add(2 * time.Hour)}

	b1 := &Block{ID: "B1", Target: NewStar("A", angle.Zero(), angle.Zero()), Duration: 10 * time.Minute}
	b2 := &Block{ID: "B2", Target: NewStar("B", angle.Zero(), angle.Zero()), Duration: 10 * time.Minute}

	placed := func(b *Block, offset time.Duration) ScheduledBlock {
		return ScheduledBlock{
			Block:  b,
			Window: Window{Start: start.Add(offset), End: start.Add(offset + 10*time.Minute)},
		}
	}

	for _, tc := range []struct {
		name  string
		sched *Schedule
	}{
		{
			name: "swapPass fails",
			sched: &Schedule{
				Window: window,
				Blocks: []ScheduledBlock{placed(b1, 0), placed(b2, 10*time.Minute)},
			},
		},
		{
			name: "insertPass fails",
			sched: &Schedule{
				Window:      window,
				Blocks:      []ScheduledBlock{placed(b1, 0)},
				Unscheduled: []UnscheduledBlock{{Block: b2}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			strategy := &SwapOptimizedStrategy{Base: seedStrategy{sched: tc.sched}, Step: time.Minute}

			_, err := strategy.Schedule(planner, window, []*Block{b1, b2}, &BasicTransitionModel{BaseSetup: 0})
			if !errors.Is(err, errConstraintUnavailable) {
				t.Errorf("Schedule returned %v, want it to wrap the constraint's error", err)
			}
		})
	}
}

// endOnlyFailingConstraint fails at one exact instant and passes everywhere
// else.
type endOnlyFailingConstraint struct{ at time.Time }

func (c endOnlyFailingConstraint) Check(_ Observable, t time.Time, _ *Site) (Result, error) {
	if t.Equal(c.at) {
		return Result{}, errConstraintUnavailable
	}

	return Result{Pass: true}, nil
}

// TestConstraintFailureAtTheExactEndIsReported covers the interval check's
// last step.
//
// checkConstraintsIntervalCtx samples from start to end and then, if the two
// differ, checks the exact end instant separately — because a step that does
// not divide the interval leaves the endpoint unsampled, and a block whose
// last moment violates a constraint is not schedulable.
//
// That extra check has its own error path, and only a constraint that fails
// exactly there reaches it: an always-failing one returns from the loop's
// first step instead.
func TestConstraintFailureAtTheExactEndIsReported(t *testing.T) {
	t.Parallel()

	site := predicateSite(t)
	start := fixedEpoch()

	// A step that does not divide the interval, so the loop never lands on end.
	end := start.Add(25 * time.Minute)

	_, ok, err := checkConstraintsIntervalCtx(
		NewStar("A", angle.Zero(), angle.Zero()),
		start, end, 10*time.Minute, site,
		endOnlyFailingConstraint{at: end},
	)

	if !errors.Is(err, errConstraintUnavailable) {
		t.Errorf("returned ok=%v err=%v, want the constraint's error from the "+
			"exact-end check", ok, err)
	}
}
