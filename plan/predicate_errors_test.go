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
// the other forty, and its doc comment says so. But every skip returned the
// same bare false as "too faint" or "never rises", so a night's list could come
// back short because a provider was down and nothing said which it was.
//
// The skip stays; it is now reported at Warn, which is the level for a result
// that is quietly less complete than it looks.
func TestVisibleTonightReportsWhyACandidateWasSkipped(t *testing.T) {
	// Not parallel: it installs the process-wide logger.
	defer logging.Set(nil)

	var buf bytes.Buffer

	logging.Set(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})))

	site := predicateSite(t)
	start := fixedEpoch()

	// A candidate whose constraint evaluation fails, driven through the same
	// helper VisibleTonight uses per candidate.
	cfg := visibleTonightConfig{step: 30 * time.Minute, minAltitude: angle.Deg(10)}

	_, ok := evaluateCandidate(
		t.Context(),
		visibleCandidate{obj: NewStar("Unreachable", angle.Zero(), angle.Zero())},
		start, start.Add(2*time.Hour), site, nil, 6.0, cfg,
	)
	if ok {
		t.Fatal("precondition: this candidate should not evaluate cleanly")
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
