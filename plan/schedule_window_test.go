package plan

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/time"
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

	site, err := NewSiteEarthLocation("Quinta Calixto", -22.528478, -46.473002, 835.05)
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
		Duration: 30 * time.Minute,
		Priority: 1,
	}}

	start := time.Date(2026, 4, 6, 22, 0, 0, 0, time.LocationUTC)

	bad := []struct {
		name   string
		window Window
		want   error
	}{
		{"reversed", Window{Start: start, End: start.Add(unit.Hours(-6))}, ErrReversedInterval},
		{"end not a number", Window{Start: start, End: time.FromJD(math.NaN(), time.UTC)}, ErrNonFiniteInterval},
	}

	strategies := []Strategy{
		&GreedyStrategy{Step: 5 * time.Minute},
		&PriorityStrategy{Step: 5 * time.Minute},
		&SwapOptimizedStrategy{Step: 5 * time.Minute},
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
