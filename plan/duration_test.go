package plan

import (
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/unit"
)

// TestNonFiniteDurationsAreRefused: plan's durations are float64 seconds, so
// a NaN or infinite one can reach a sampling loop or a schedule where a
// time.Duration could not, and a comparison with zero lets NaN through. Each
// entry point refuses it with ErrNonFiniteDuration rather than sampling a NaN
// instant, placing a block at one, or taking it for unset.
func TestNonFiniteDurationsAreRefused(t *testing.T) {
	t.Parallel()

	nan, inf := unit.Duration(math.NaN()), unit.Duration(math.Inf(1))
	site, start := predicateSite(t), fixedEpoch()
	end := start.Add(unit.Hours(2))
	star := NewStar("A", angle.Zero(), angle.Zero())
	fixed := benchMock{c: coord.NewICRS(angle.Zero(), angle.Zero())}

	schedule := func(s Strategy, tm TransitionModel, edit func(b1, b2 *Block)) error {
		planner, window, b1, b2 := overheadFixture(t)
		if edit != nil {
			edit(b1, b2)
		}

		if _, err := s.Schedule(planner, window, []*Block{b1, b2}, tm); err != nil {
			return fmt.Errorf("schedule: %w", err)
		}

		return nil
	}

	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{"a NaN GreedyStrategy.Step", func() error {
			return schedule(&GreedyStrategy{Step: nan}, &BasicTransitionModel{}, nil)
		}},
		// With a Base of its own, which does not see the swap passes' step.
		{"an infinite SwapOptimizedStrategy.Step", func() error {
			return schedule(&SwapOptimizedStrategy{Step: inf, Base: &GreedyStrategy{}}, &BasicTransitionModel{}, nil)
		}},
		{"a NaN Block.Duration", func() error {
			return schedule(&GreedyStrategy{}, &BasicTransitionModel{}, func(b1, _ *Block) { b1.Duration = nan })
		}},
		{"an infinite Cadence.MinInterval", func() error {
			return schedule(&PriorityStrategy{}, &BasicTransitionModel{}, func(_, b2 *Block) {
				b2.Cadence = &Cadence{MinInterval: inf, Repeats: 1}
			})
		}},
		{"a NaN BaseSetup", func() error {
			_, err := (&BasicTransitionModel{BaseSetup: nan}).Overhead(Transition{})

			return err
		}},
		{"an infinite FilterChangePenalty", func() error {
			_, err := (&BasicTransitionModel{FilterChangePenalty: inf}).Overhead(Transition{})

			return err
		}},
		{"a NaN VisibleIntervals step", func() error {
			_, err := VisibleIntervals(fixed, site, start, end, nan, angle.Zero())

			return err
		}},
		{"a NaN Find step", func() error {
			_, err := Find(fixed, site, nil, start, end, nan)

			return err
		}},
		{"a NaN ObservableWindows step", func() error {
			_, err := ObservableWindows(star, start, end, nan, site)

			return err
		}},
		{"a NaN EventSolver.Step", func() error {
			_, err := EventSolver{Step: nan}.Find(EventSpec{
				Family: EventFamilyVisibility, Kind: EventAnyVisibility, Target: star, Observer: site,
			}, start, end)

			return err
		}},
	} {
		if err := tc.run(); !errors.Is(err, ErrNonFiniteDuration) {
			t.Errorf("%s: got %v, want %v", tc.name, err, ErrNonFiniteDuration)
		}
	}
}

// TestEventSolverDefaultsAnUnsetStep: NewEventSolver returns no error, so a
// step or tolerance that is not a positive number of seconds, NaN included,
// takes the default. A solver built as a literal has no defaults, and Find
// refuses its zero step.
func TestEventSolverDefaultsAnUnsetStep(t *testing.T) {
	t.Parallel()

	for _, v := range []unit.Duration{0, -1, unit.Duration(math.NaN()), unit.Duration(math.Inf(1))} {
		s := NewEventSolver(v, v)
		if s.Step != unit.Minutes(15) || s.Solver.Tolerance != unit.Seconds(1) {
			t.Errorf("NewEventSolver(%v, %v): step %v, tolerance %v, want 15 min and 1 s", v, v, s.Step, s.Solver.Tolerance)
		}
	}

	site, start := predicateSite(t), fixedEpoch()

	_, err := EventSolver{}.Find(EventSpec{
		Family: EventFamilyVisibility, Kind: EventAnyVisibility,
		Target: NewStar("A", angle.Zero(), angle.Zero()), Observer: site,
	}, start, start.Add(unit.Hours(1)))
	if !errors.Is(err, ErrStepNotPositive) {
		t.Errorf("a literal solver's zero step: got %v, want %v", err, ErrStepNotPositive)
	}
}

// TestWindowDurationIsSeconds: a window's Duration is the unit.Duration
// Time.Sub gives, and TotalDuration counts an overlap once.
func TestWindowDurationIsSeconds(t *testing.T) {
	t.Parallel()

	start := fixedEpoch()
	at := func(h float64) Window { return Window{Start: start, End: start.Add(unit.Hours(h))} }

	if d := at(2).Duration(); math.Abs(d.Seconds()-7200) > 1e-3 {
		t.Errorf("a two-hour window lasts %v s, want 7200", d.Seconds())
	}

	later := Window{Start: start.Add(unit.Hours(1)), End: start.Add(unit.Hours(3))}
	if d := TotalDuration([]Window{at(2), later}); math.Abs(d.Hours()-3) > 1e-6 {
		t.Errorf("0-2 h and 1-3 h total %v h, want 3", d.Hours())
	}
}
