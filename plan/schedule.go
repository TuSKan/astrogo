package plan

import (
	"fmt"
	"math"
	"sort"

	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// Configuration represents the setup needed for an observation block.
// This can include instrument settings, filters, readout modes, etc.
type Configuration struct {
	Filter     string
	Instrument string
}

// Cadence specifies recurrence requirements for an observation block.
type Cadence struct {
	MinInterval time.Duration // Minimum time to wait before re-observing
	Repeats     int           // Number of additional times to repeat this block (e.g. 1 means observe 2 times total)
}

// Block represents a single request to observe a Target for a specified Duration.
// It includes priority and constraints specific to this request.
type Block struct {
	Target      Observable
	Cadence     *Cadence
	Config      Configuration
	ID          string
	Constraints []Constraint
	Duration    time.Duration
	Priority    float64
}

// ScheduledBlock represents a block that has been successfully assigned a time window.
type ScheduledBlock struct {
	Block     *Block
	Window    Window
	Score     float64
	SetupTime time.Duration // Time spent on setup/slew before exposing
}

// UnscheduledBlock represents a block that could not be scheduled.
type UnscheduledBlock struct {
	Block  *Block
	Reason string
}

// Schedule is the final generated observation timeline.
type Schedule struct {
	Site        *Site
	Window      Window
	Blocks      []ScheduledBlock
	Unscheduled []UnscheduledBlock
}

// Transition is what a [TransitionModel] prices: the move from one
// observation to the next.
//
// It was TransitionContext, which was not a coord.Context. It carried a
// hidden pointer to the strategy's Context cache, so that a model could reach
// a Context through its ContextAt method without building one per call
// (#485). What a model needs is where the two targets are, so the strategy
// now puts that in FromAltAz and ToAltAz, observed through the Context it
// evaluates constraints with, and a model needs no Context at all (#675).
type Transition struct {
	FromBlock *Block // nil if ToBlock is the first block
	ToBlock   *Block
	FromTime  time.Time // when the previous observation ended
	ToTime    time.Time // when the next observation begins (approximate, often FromTime)
	Site      *Site

	// FromAltAz is FromBlock's target observed from Site at FromTime, where
	// the telescope points as the move begins; zero when FromBlock is nil.
	// ToAltAz is ToBlock's target at ToTime, where it has to point.
	FromAltAz, ToAltAz coord.AltAz
}

// NewTransition is the Transition from one block to the next, as a strategy
// hands it to a [TransitionModel]: each target observed from site at its own
// instant. It builds a coord.Context for the pair, as Overhead used to for
// every call; the built-in strategies observe both through the Context they
// evaluate constraints with instead.
//
// An error means a target could not be observed, and names which end.
func NewTransition(from, to *Block, fromTime, toTime time.Time, site *Site) (Transition, error) {
	return newTransition(from, to, fromTime, toTime, site, movingContext(site.Location(), site.Refraction()))
}

// newTransition is NewTransition with the Context for each instant supplied
// by ctxAt. ctxAt moves one Context, so each position is taken before the
// next instant is asked for.
func newTransition(
	from, to *Block, fromTime, toTime time.Time, site *Site, ctxAt func(time.Time) *coord.Context,
) (Transition, error) {
	tr := Transition{FromBlock: from, ToBlock: to, FromTime: fromTime, ToTime: toTime, Site: site}

	if from != nil {
		aa, err := observedTargetAt(from.Target, fromTime, ctxAt)
		if err != nil {
			return Transition{}, fmt.Errorf("schedule: from %w", err)
		}

		tr.FromAltAz = aa
	}

	aa, err := observedTargetAt(to.Target, toTime, ctxAt)
	if err != nil {
		return Transition{}, fmt.Errorf("schedule: to %w", err)
	}

	tr.ToAltAz = aa

	return tr, nil
}

// observedTargetAt is target's observed alt/az at t, through ctxAt's Context.
func observedTargetAt(target Observable, t time.Time, ctxAt func(time.Time) *coord.Context) (coord.AltAz, error) {
	pos, err := target.Position(t)
	if err != nil {
		return coord.AltAz{}, fmt.Errorf("position: %w", err)
	}

	aa, err := observedAltAz(target, t, ctxAt(t), pos)
	if err != nil {
		return coord.AltAz{}, fmt.Errorf("AltAz: %w", err)
	}

	return aa, nil
}

// TransitionModel evaluates the overhead of moving between two observations.
//
// An error fails the schedule being built: every strategy returns it, naming
// the two blocks, rather than skipping the candidate. Return one only when
// the overhead cannot be known; a transition that is merely expensive is a
// long duration.
type TransitionModel interface {
	Overhead(tr Transition) (time.Duration, error)
}

// BasicTransitionModel provides a fundamental slew and configuration penalty model.
type BasicTransitionModel struct {
	// BaseSetup is the default overhead applied when initializing pointing
	// if there is no previous block.
	BaseSetup time.Duration

	// SlewRate is the dome/mount slew speed in degrees per second.
	SlewRate float64

	// FilterChangePenalty is the time taken to change filters.
	FilterChangePenalty time.Duration
}

// Overhead calculates the transition time using separation in Alt/Az at the given times.
func (m *BasicTransitionModel) Overhead(tr Transition) (time.Duration, error) {
	// Initial pointing initialization
	if tr.FromBlock == nil {
		setup := m.BaseSetup
		if setup <= 0 {
			setup = 1 * time.Minute
		}

		return setup, nil
	}

	var total time.Duration

	// Configuration overhead
	if tr.FromBlock.Config.Filter != tr.ToBlock.Config.Filter && tr.FromBlock.Config.Filter != "" && tr.ToBlock.Config.Filter != "" {
		total += m.FilterChangePenalty
	}

	// Slew Time
	if m.SlewRate > 0 {
		// Calculate separation on Alt and Az independently.
		// Assuming simultaneous slew on two independent axes, slew time is
		// determined by the axis that takes the longest.
		dAlt := math.Abs(tr.FromAltAz.Alt().Degrees() - tr.ToAltAz.Alt().Degrees())

		azFrom := tr.FromAltAz.Az().Degrees()
		azTo := tr.ToAltAz.Az().Degrees()

		dAz := math.Abs(azFrom - azTo)
		if dAz > 180.0 {
			dAz = 360.0 - dAz
		}

		maxAngularDist := math.Max(dAlt, dAz)
		slewSeconds := maxAngularDist / m.SlewRate

		// Add slew time
		total += time.Duration(slewSeconds * float64(time.Second))
	}

	return total, nil
}

// Strategy provides an algorithm to map a list of Blocks to a Schedule.
//
// This is the primary extension point for custom scheduling algorithms.
// The built-in strategies (GreedyStrategy, PriorityStrategy, SwapOptimizedStrategy)
// use local heuristics. Users requiring global optimization (e.g., integer
// linear programming, simulated annealing, genetic algorithms) should implement
// this interface directly.
//
// Performance note: the built-in strategies evaluate constraints through one
// coord.Context, moved to each step with coord.Context.SetTime rather than
// building a new one (~91 µs). A custom Strategy evaluating many instants can
// do the same.
type Strategy interface {
	// Schedule produces a Schedule from the provided Blocks within the given Window.
	// The implementation should use Planner for constraint evaluation and TransitionModel
	// for overhead calculation.
	Schedule(planner *Planner, window Window, blocks []*Block, transition TransitionModel) (*Schedule, error)
}

// Scheduler orchestrates the scheduling of Blocks according to a specific Strategy.
type Scheduler struct {
	Planner         *Planner
	Strategy        Strategy
	TransitionModel TransitionModel
}

// NewScheduler creates a new Scheduler using the specified Planner, Strategy, and TransitionModel.
func NewScheduler(planner *Planner, strategy Strategy, tm TransitionModel) *Scheduler {
	return &Scheduler{
		Planner:         planner,
		Strategy:        strategy,
		TransitionModel: tm,
	}
}

// BuildSchedule generates a Schedule for the provided blocks within the given window.
func (s *Scheduler) BuildSchedule(window Window, blocks []*Block) (*Schedule, error) {
	if s.Strategy == nil {
		return &Schedule{
			Site:   s.Planner.Site,
			Window: window,
		}, nil
	}

	sched, err := s.Strategy.Schedule(s.Planner, window, blocks, s.TransitionModel)
	if err != nil {
		return nil, fmt.Errorf("scheduler: strategy: %w", err)
	}

	return sched, nil
}

const defaultStep = 1 * time.Minute

// overheadError names the transition whose overhead could not be computed.
//
// Every strategy used to skip such a candidate, so a block whose target could
// not be positioned for the slew never placed and nothing said why; and the
// greedy pass refined its estimate with the error discarded, placing the block
// with no setup time at all. A failed overhead is the same failure as a
// constraint that cannot be evaluated for that target, which already fails the
// schedule, so it does too (#454).
func overheadError(from, to *Block, err error) error {
	fromID := "the start"
	if from != nil {
		fromID = from.ID
	}

	return fmt.Errorf("transition from %s to %s: %w", fromID, to.ID, err)
}

// checkConstraintsInterval verifies that all constraints pass continuously
// over a time range.
//
// The Context at each step comes from ctxAt, which a strategy builds once per
// Schedule with movingContext, and is shared across the constraints that
// implement ConstraintCtx. It was a full coord.NewContext per step, per
// candidate placement: 99.5% of a greedy schedule's time (#481).
//
// It used to return the Context of the step closest to the interval's
// midpoint, for scoreBlockPlacement to score the block at the midpoint with.
// That Context was turned to the step's Earth rotation, not the midpoint's,
// and they differ whenever the block is not an even number of steps long; the
// score now takes ctxAt(mid) itself, which the moving Context makes cheap.
//
// # A constraint that fails to evaluate is an error, not a "no"
//
// This used to read `if err != nil || !res.Pass { return false }`, so a
// constraint that could not be evaluated — an ephemeris lookup that failed, a
// provider that could not be reached — was indistinguishable from a constraint
// the target genuinely failed. The block was then dropped and the caller got a
// schedule that looked complete.
//
// Constraint.Check returns an error precisely because a check can fail rather
// than merely be false, and discarding that at the last step is the shape of
// #102, where a swallowed error turned a CDS outage into "target not found".
// The error now propagates: a caller who cannot evaluate their constraints
// should hear about it rather than receive a quietly shorter plan.
func checkConstraintsInterval(
	target Observable,
	start, end time.Time,
	step time.Duration,
	site *Site,
	ctxAt func(time.Time) *coord.Context,
	constraints ...Constraint,
) (bool, error) {
	check := func(t time.Time) (bool, error) {
		ctx := ctxAt(t)

		for _, c := range constraints {
			var (
				res Result
				err error
			)
			if cc, ok := c.(ConstraintCtx); ok {
				res, err = cc.CheckCtx(target, t, site, ctx)
			} else {
				res, err = c.Check(target, t, site)
			}

			if err != nil {
				return false, fmt.Errorf("plan: constraint at %s: %w",
					t.Format(time.RFC3339), err)
			}

			if !res.Pass {
				return false, nil
			}
		}

		return true, nil
	}

	t := start
	for t.Before(end) || t.Equal(end) {
		ok, err := check(t)
		if err != nil || !ok {
			return false, err
		}

		t = t.Add(time.FromGoDuration(step))
	}

	// Always check the exact end time as well.
	if !start.Equal(end) {
		return check(end)
	}

	return true, nil
}

// GreedyStrategy traverses time forward and schedules the first block in the list
// that satisfies all constraints at the given time. It results in a dense schedule.
type GreedyStrategy struct {
	// Step is the time increment used when searching for a valid start time.
	Step time.Duration
}

// Schedule implements Strategy for GreedyStrategy.
func (s *GreedyStrategy) Schedule(planner *Planner, window Window, blocks []*Block, transition TransitionModel) (*Schedule, error) {
	return s.schedule(planner, window, blocks, transition, movingContext(planner.Site.Location(), planner.Site.Refraction()))
}

// schedule is Schedule with the Context for each instant supplied by ctxAt,
// which lets a test run the same pass on a full Context per instant.
func (s *GreedyStrategy) schedule(
	planner *Planner,
	window Window,
	blocks []*Block,
	transition TransitionModel,
	ctxAt func(time.Time) *coord.Context,
) (*Schedule, error) {
	step := s.Step
	if step <= 0 {
		step = defaultStep
	}

	sched := &Schedule{
		Site:   planner.Site,
		Window: window,
	}

	currentTime := window.Start

	var lastBlock *Block

	type activeItem struct {
		b         *Block
		available time.Time
		rem       int
	}

	var unassigned []*activeItem

	for _, b := range blocks {
		repeats := 0
		if b.Cadence != nil {
			repeats = b.Cadence.Repeats
		}

		unassigned = append(unassigned, &activeItem{
			b:         b,
			available: window.Start,
			rem:       repeats,
		})
	}

	for currentTime.Before(window.End) && len(unassigned) > 0 {
		placed := false

		for i := 0; i < len(unassigned); i++ {
			item := unassigned[i]
			b := item.b

			if currentTime.Before(item.available) {
				continue
			}

			// Calculate transition overhead, with the destination time
			// approximated by the current time first.
			overhead, err := transitionOverhead(transition, lastBlock, b, currentTime, currentTime, planner.Site, ctxAt)
			if err != nil {
				return nil, fmt.Errorf("plan: greedy: %w", err)
			}

			// Refine Transition Overhead with better approximation of destination time
			overhead, err = transitionOverhead(transition, lastBlock, b,
				currentTime, currentTime.Add(time.FromGoDuration(overhead)), planner.Site, ctxAt)
			if err != nil {
				return nil, fmt.Errorf("plan: greedy: %w", err)
			}

			startTime := currentTime.Add(time.FromGoDuration(overhead))
			endTime := startTime.Add(time.FromGoDuration(b.Duration))

			if endTime.After(window.End) {
				continue // Block execution exceeds the scheduling window
			}

			// Combine base planner constraints with block-specific constraints
			allConstraints := append(make([]Constraint, 0, len(planner.Constraints)+len(b.Constraints)), planner.Constraints...)
			allConstraints = append(allConstraints, b.Constraints...)

			// Check observability over the full duration
			ok, err := checkConstraintsInterval(b.Target, startTime, endTime, step, planner.Site, ctxAt, allConstraints...)
			if err != nil {
				return nil, fmt.Errorf("plan: greedy: block %s: %w", b.ID, err)
			}

			if ok {
				score := scoreBlockPlacement(b, startTime, endTime, planner, ctxAt)
				sched.Blocks = append(sched.Blocks, ScheduledBlock{
					Block:     b,
					Window:    Window{Start: startTime, End: endTime},
					SetupTime: overhead,
					Score:     score,
				})

				currentTime = endTime
				lastBlock = b

				if item.rem > 0 {
					item.rem--
					// Next observation can start after MinInterval passes
					item.available = endTime.Add(time.FromGoDuration(b.Cadence.MinInterval))
				} else {
					unassigned = append(unassigned[:i], unassigned[i+1:]...)
				}

				placed = true

				break
			}
		}

		if !placed {
			// Time gap: no block could be scheduled, advance time

			// Optimization: if all remaining items are waiting for cadence, fast-forward time
			allWaiting := true
			earliestAvailable := window.End

			for _, item := range unassigned {
				if !currentTime.Before(item.available) {
					allWaiting = false
					break
				}

				if item.available.Before(earliestAvailable) {
					earliestAvailable = item.available
				}
			}

			if allWaiting && earliestAvailable.After(currentTime) {
				currentTime = earliestAvailable
			} else {
				currentTime = currentTime.Add(time.FromGoDuration(step))
			}
		}
	}

	for _, item := range unassigned {
		sched.Unscheduled = append(sched.Unscheduled, UnscheduledBlock{
			Block:  item.b,
			Reason: "constraints unsatisfied, insufficient time, or cadence unfulfillable in window",
		})
	}

	return sched, nil
}

// PriorityStrategy pre-sorts blocks by Priority (descending) before applying
// a time-forward greedy allocation, ensuring high-priority blocks win time slots.
type PriorityStrategy struct {
	Step time.Duration
}

// Schedule implements Strategy for PriorityStrategy.
func (s *PriorityStrategy) Schedule(planner *Planner, window Window, blocks []*Block, transition TransitionModel) (*Schedule, error) {
	sorted := make([]*Block, len(blocks))
	copy(sorted, blocks)

	// Sort explicitly by priority descending
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Priority > sorted[j].Priority
	})

	greedy := GreedyStrategy{Step: s.Step}

	return greedy.Schedule(planner, window, sorted, transition)
}
