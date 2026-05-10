package plan

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// mockConstraint always passes or fails based on its field
type mockConstraint struct {
	pass bool
}

func (m mockConstraint) Check(target Observable, t time.Time, site *Site) (Result, error) {
	return Result{Pass: m.pass}, nil
}

func TestSchedulerAndStrategies(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)

	site, err := NewSite("TestSite", loc, angle.Zero(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	planner, _ := NewPlanner(site, nil)

	tm := &BasicTransitionModel{BaseSetup: 0} // Simplify time math by zeroing base setup

	start := time.ZeroTime()
	window := Window{Start: start, End: start.Add(1 * time.Hour)}

	b1 := &Block{ID: "B1", Target: NewStar("T", angle.Zero(), angle.Zero()), Duration: 10 * time.Minute}
	b2 := &Block{ID: "B2", Target: NewStar("T", angle.Zero(), angle.Zero()), Duration: 20 * time.Minute, Priority: 5.0}

	// 1. Greedy Strategy
	scheduler := NewScheduler(planner, &GreedyStrategy{}, tm)

	sched, err := scheduler.BuildSchedule(window, []*Block{b1, b2})
	if err != nil {
		t.Fatalf("Greedy scheduling failed: %v", err)
	}

	if len(sched.Blocks) != 2 || len(sched.Unscheduled) != 0 {
		t.Fatalf("expected 2 scheduled, 0 unscheduled, got %d, %d", len(sched.Blocks), len(sched.Unscheduled))
	}

	// Greedy honors input order
	if sched.Blocks[0].Block.ID != "B1" {
		t.Errorf("expected B1 first, got %s", sched.Blocks[0].Block.ID)
	}

	// 2. Priority Strategy
	scheduler = NewScheduler(planner, &PriorityStrategy{}, tm)

	sched, err = scheduler.BuildSchedule(window, []*Block{b1, b2})
	if err != nil {
		t.Fatalf("Priority scheduling failed: %v", err)
	}

	// Priority honors Priority field (B2 > B1)
	if sched.Blocks[0].Block.ID != "B2" {
		t.Errorf("expected B2 first due to priority, got %s", sched.Blocks[0].Block.ID)
	}

	// 3. Constraints logic
	b3 := &Block{
		ID:          "B3",
		Target:      NewStar("T3", angle.Zero(), angle.Zero()),
		Duration:    20 * time.Minute,
		Constraints: []Constraint{mockConstraint{pass: false}},
	}

	sched, err = scheduler.BuildSchedule(window, []*Block{b3})
	if err != nil {
		t.Fatalf("Constraint scheduling failed: %v", err)
	}

	if len(sched.Blocks) != 0 || len(sched.Unscheduled) != 1 {
		t.Errorf("expected B3 to fail constraint check")
	}
}
