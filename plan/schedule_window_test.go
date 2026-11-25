package plan

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/unit"
)

// TestScheduleRefusesABadWindow: a window that ends before it starts, or at a
// time that is not a number, is refused with the interval sentinels every other
// plan search uses, by BuildSchedule and by each strategy called directly. An
// empty window stays valid. Until #1177 all three strategies returned a nil
// error and listed every block as unscheduled, which reads as a night when
// nothing could be observed.
func TestScheduleRefusesABadWindow(t *testing.T) {
	t.Parallel()

	site, err := NewSite("Quinta Calixto", angle.LatDeg(-22.528478), angle.LonDeg(-46.473002), unit.Meters(835.05))
	if err != nil {
		t.Fatal(err)
	}

	planner, err := NewPlanner(site, nil)
	if err != nil {
		t.Fatal(err)
	}

	blocks := []*Block{{
		ID:       "alpha Cen",
		Target:   NewStar("alpha Cen", angle.Hour(14.66), angle.Deg(-60.83)),
		Duration: unit.Minutes(30),
		Priority: 1,
	}}

	start := epoch.Date(2026, 4, 6, 22, 0, 0, 0, time.UTC)

	bad := []struct {
		name   string
		window Window
		want   error
	}{
		{"reversed", Window{Start: start, End: start.Add(unit.Hours(-6))}, ErrReversedInterval},
		{"end not a number", Window{Start: start, End: epoch.FromJD(math.NaN(), epoch.UTC)}, ErrNonFiniteInterval},
	}

	strategies := []Strategy{
		&GreedyStrategy{Step: unit.Minutes(5)},
		&PriorityStrategy{Step: unit.Minutes(5)},
		&SwapOptimizedStrategy{Step: unit.Minutes(5)},
	}

	for _, st := range strategies {
		for _, c := range bad {
			sched, err := NewScheduler(planner, st, &BasicTransitionModel{}).BuildSchedule(c.window, blocks)
			if !errors.Is(err, c.want) || sched != nil {
				t.Errorf("%T BuildSchedule, %s window: schedule %v, err %v; want none and %v", st, c.name, sched, err, c.want)
			}

			sched, err = st.Schedule(planner, c.window, blocks, &BasicTransitionModel{})
			if !errors.Is(err, c.want) || sched != nil {
				t.Errorf("%T Schedule, %s window: schedule %v, err %v; want none and %v", st, c.name, sched, err, c.want)
			}
		}

		if _, err := NewScheduler(planner, st, &BasicTransitionModel{}).BuildSchedule(Window{Start: start, End: start}, blocks); err != nil {
			t.Errorf("%T: an empty window was refused: %v", st, err)
		}
	}

	// No strategy at all still checks the window it is handed.
	if _, err := NewScheduler(planner, nil, nil).BuildSchedule(bad[0].window, blocks); !errors.Is(err, ErrReversedInterval) {
		t.Errorf("nil strategy: err = %v, want ErrReversedInterval", err)
	}
}
