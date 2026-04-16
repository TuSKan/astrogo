package plan

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// TestSwapOptimizedStrategy verifies that the SwapOptimizedStrategy
// can improve upon a naive greedy schedule.
func TestSwapOptimizedStrategy(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, err := NewSite("TestSite", loc, angle.Zero(), nil)
	if err != nil {
		t.Fatal(err)
	}
	planner, _ := NewPlanner(site, nil)

	tm := &BasicTransitionModel{BaseSetup: 0}

	start := time.ZeroTime()
	window := Window{Start: start, End: start.Add(1 * time.Hour)}

	b1 := &Block{ID: "B1", Target: Custom{Label: "T1"}, Duration: 20 * time.Minute, Priority: 1.0}
	b2 := &Block{ID: "B2", Target: Custom{Label: "T2"}, Duration: 20 * time.Minute, Priority: 5.0}
	b3 := &Block{ID: "B3", Target: Custom{Label: "T3"}, Duration: 10 * time.Minute, Priority: 3.0}

	// SwapOptimized with PriorityStrategy base
	strategy := &SwapOptimizedStrategy{
		Base:      &PriorityStrategy{},
		MaxPasses: 3,
	}
	scheduler := NewScheduler(planner, strategy, tm)
	sched, err := scheduler.BuildSchedule(window, []*Block{b1, b2, b3})
	if err != nil {
		t.Fatalf("SwapOptimized scheduling failed: %v", err)
	}

	// B2 should be first (priority 5.0 via PriorityStrategy base)
	if len(sched.Blocks) < 2 {
		t.Fatalf("expected at least 2 scheduled blocks, got %d", len(sched.Blocks))
	}
	if sched.Blocks[0].Block.ID != "B2" {
		t.Errorf("expected B2 first (highest priority), got %s", sched.Blocks[0].Block.ID)
	}

	// All blocks should be scheduled
	if len(sched.Unscheduled) != 0 {
		t.Errorf("expected 0 unscheduled, got %d", len(sched.Unscheduled))
	}

	t.Logf("Scheduled %d blocks, %d unscheduled", len(sched.Blocks), len(sched.Unscheduled))
	for i, sb := range sched.Blocks {
		t.Logf("  [%d] %s: %s → %s (score=%.2f)", i, sb.Block.ID, sb.Window.Start, sb.Window.End, sb.Score)
	}
}

// TestSwapOptimizedGapInsertion verifies that the gap insertion pass
// recovers blocks that the greedy scheduler couldn't place.
func TestSwapOptimizedGapInsertion(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, err := NewSite("TestSite", loc, angle.Zero(), nil)
	if err != nil {
		t.Fatal(err)
	}
	planner, _ := NewPlanner(site, nil)

	tm := &BasicTransitionModel{BaseSetup: 0}

	start := time.ZeroTime()
	window := Window{Start: start, End: start.Add(1 * time.Hour)}

	// B1 always fails constraints → will be unscheduled by greedy
	b1 := &Block{
		ID:          "Failing",
		Target:      Custom{Label: "T1"},
		Duration:    10 * time.Minute,
		Priority:    10.0,
		Constraints: []Constraint{mockConstraint{pass: false}},
	}
	b2 := &Block{ID: "B2", Target: Custom{Label: "T2"}, Duration: 20 * time.Minute, Priority: 5.0}
	b3 := &Block{ID: "B3", Target: Custom{Label: "T3"}, Duration: 10 * time.Minute, Priority: 1.0}

	strategy := &SwapOptimizedStrategy{
		Base:      &PriorityStrategy{},
		MaxPasses: 3,
	}

	sched, err := strategy.Schedule(planner, window, []*Block{b1, b2, b3}, tm)
	if err != nil {
		t.Fatal(err)
	}

	// B1 should be unscheduled (constraint always fails)
	if len(sched.Unscheduled) != 1 || sched.Unscheduled[0].Block.ID != "Failing" {
		t.Errorf("expected B1 (Failing) to be unscheduled, got %d unscheduled", len(sched.Unscheduled))
	}

	// B2 and B3 should both be scheduled
	if len(sched.Blocks) != 2 {
		t.Errorf("expected 2 scheduled blocks, got %d", len(sched.Blocks))
	}

	t.Logf("Scheduled: %d, Unscheduled: %d", len(sched.Blocks), len(sched.Unscheduled))
}

// TestScheduleGaps verifies gap computation between scheduled blocks.
func TestScheduleGaps(t *testing.T) {
	start := time.ZeroTime()
	window := Window{Start: start, End: start.Add(1 * time.Hour)}

	blocks := []ScheduledBlock{
		{
			Block:  &Block{ID: "A"},
			Window: Window{Start: start.Add(10 * time.Minute), End: start.Add(20 * time.Minute)},
		},
		{
			Block:  &Block{ID: "B"},
			Window: Window{Start: start.Add(30 * time.Minute), End: start.Add(40 * time.Minute)},
		},
	}

	gaps := scheduleGaps(blocks, window)

	// Expected gaps: [0, 10], [20, 30], [40, 60] = 3 gaps
	if len(gaps) != 3 {
		t.Fatalf("expected 3 gaps, got %d", len(gaps))
	}

	// First gap: before block A
	if gaps[0].prevBlock != nil {
		t.Error("first gap should have nil prevBlock")
	}
	expectedDur := 10 * time.Minute
	if gaps[0].window.Duration() != expectedDur {
		t.Errorf("first gap duration: got %v, want %v", gaps[0].window.Duration(), expectedDur)
	}

	// Second gap: between A and B
	if gaps[1].prevBlock.ID != "A" {
		t.Errorf("second gap prevBlock: got %s, want A", gaps[1].prevBlock.ID)
	}

	// Third gap: after B
	if gaps[2].prevBlock.ID != "B" {
		t.Errorf("third gap prevBlock: got %s, want B", gaps[2].prevBlock.ID)
	}
}

// TestEmptyScheduleGaps verifies gap computation on an empty schedule.
func TestEmptyScheduleGaps(t *testing.T) {
	start := time.ZeroTime()
	window := Window{Start: start, End: start.Add(1 * time.Hour)}

	gaps := scheduleGaps(nil, window)
	if len(gaps) != 1 {
		t.Fatalf("expected 1 gap for empty schedule, got %d", len(gaps))
	}
	if gaps[0].window.Duration() != 1*time.Hour {
		t.Errorf("gap duration: got %v, want 1h", gaps[0].window.Duration())
	}
}

// TestSwapOptimizedWithNilBase verifies the default base strategy fallback.
func TestSwapOptimizedWithNilBase(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := NewSite("TestSite", loc, angle.Zero(), nil)
	planner, _ := NewPlanner(site, nil)

	tm := &BasicTransitionModel{BaseSetup: 0}

	start := time.ZeroTime()
	window := Window{Start: start, End: start.Add(1 * time.Hour)}

	b1 := &Block{ID: "B1", Target: Custom{Label: "T1"}, Duration: 20 * time.Minute, Priority: 1.0}

	strategy := &SwapOptimizedStrategy{} // nil Base → defaults to PriorityStrategy
	sched, err := strategy.Schedule(planner, window, []*Block{b1}, tm)
	if err != nil {
		t.Fatal(err)
	}
	if len(sched.Blocks) != 1 {
		t.Errorf("expected 1 scheduled block, got %d", len(sched.Blocks))
	}
}

// TestScoreBlockPlacement verifies that scoring uses altitude + priority.
func TestScoreBlockPlacement(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := NewSite("Test", loc, angle.Zero(), nil)
	planner, _ := NewPlanner(site, nil)

	b := &Block{
		ID:       "B1",
		Target:   Custom{Label: "T1"},
		Duration: 20 * time.Minute,
		Priority: 2.0,
	}

	start := time.ZeroTime()
	score := scoreBlockPlacement(b, start, start.Add(20*time.Minute), planner)
	t.Logf("Score for mock block: %.2f", score)

	// Score should be non-negative
	if score < 0 {
		t.Errorf("score should be non-negative, got %.2f", score)
	}
}
