package plan

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/unit"
)

// Configuration represents the setup needed for an observation block.
// This can include instrument settings, filters, readout modes, etc.
type Configuration struct {
	Filter     string
	Instrument string
}

// Cadence specifies recurrence requirements for an observation block.
type Cadence struct {
	MinInterval unit.Duration // Minimum time to wait before re-observing
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
	Duration    unit.Duration
	Priority    float64
}

// ScheduledBlock represents a block that has been successfully assigned a time window.
type ScheduledBlock struct {
	Block     *Block
	Window    Window
	Score     float64
	SetupTime unit.Duration // Time spent on setup/slew before exposing
}

// UnscheduledBlock is a block that could not be scheduled, and why: Cause
// says what kind of reason, Constraint names the constraint that refused it,
// and Reason says it in words.
//
// The reason is the one the strategy's forward pass found: for
// [SwapOptimizedStrategy], its first, greedy pass, whose verdict its later
// passes did not overturn.
type UnscheduledBlock struct {
	Block *Block
	// Constraint is the constraint that rejected the most placements of the
	// block, the first of them on a tie: the one that kept it out, when Cause
	// is CauseConstraint. For CauseCadence it is the one that rejected the
	// most placements, if any did. It is nil when none rejected one.
	Constraint Constraint
	// Reason says why in words: the cause, the constraint with how many of
	// the placements tried it rejected, and the constraint's own reason the
	// last time it rejected one, such as "target is below the horizon".
	Reason string
	// Cause is what kept the block out.
	Cause UnscheduledCause
	// Observed is how many times the block was placed: for CauseCadence, the
	// observations made before the window ran out for the rest; zero
	// otherwise.
	Observed int
}

// UnscheduledCause is what kept a block out of a schedule.
type UnscheduledCause int

const (
	// CauseNoTime is a block that never fit: the block, with the slew to it,
	// was longer than the time the window had left at every instant the
	// strategy tried, so no placement of it reached its constraints.
	CauseNoTime UnscheduledCause = iota + 1
	// CauseConstraint is a block whose every placement that fit in the
	// window failed a constraint. UnscheduledBlock.Constraint names the one
	// that failed most often.
	CauseConstraint
	// CauseCadence is a block observed fewer times than its Cadence asks:
	// the window ended before the remaining repeats could be placed.
	CauseCadence
)

// String names the cause.
func (c UnscheduledCause) String() string {
	switch c {
	case CauseNoTime:
		return "no time"
	case CauseConstraint:
		return "constraint"
	case CauseCadence:
		return "cadence"
	default:
		return fmt.Sprintf("UnscheduledCause(%d)", int(c))
	}
}

// Schedule is the final generated observation timeline.
type Schedule struct {
	Site        *Site
	Window      Window
	Blocks      []ScheduledBlock
	Unscheduled []UnscheduledBlock
}

// Transition is what a [TransitionModel] prices: the move from one
// observation to the next. It carries where the two targets are, in FromAltAz
// and ToAltAz, observed through the Context the strategy evaluates
// constraints with, so a model needs no Context of its own.
type Transition struct {
	FromBlock *Block // nil if ToBlock is the first block
	ToBlock   *Block
	FromTime  epoch.Time // when the previous observation ended
	ToTime    epoch.Time // when the next observation begins (approximate, often FromTime)
	Site      *Site

	// FromAltAz is FromBlock's target observed from Site at FromTime, where
	// the telescope points as the move begins; zero when FromBlock is nil.
	// ToAltAz is ToBlock's target at ToTime, where it has to point.
	FromAltAz, ToAltAz coord.AltAz
}

// NewTransition is the Transition from one block to the next, as a strategy
// hands it to a [TransitionModel]: each target observed from site at its own
// instant, from's at w.Start, when its observation ends, and to's at w.End,
// when the next begins. It builds a coord.Context for the pair; the built-in strategies
// observe both through the Context they evaluate constraints with instead.
//
// An error means a target could not be observed, and names which end.
func NewTransition(from, to *Block, site *Site, w Window) (Transition, error) {
	fromTime, toTime := w.Start, w.End

	if site == nil {
		return Transition{}, ErrNilSite
	}

	ctxAt, err := movingContext(fromTime, site.Location(), site.Refraction())
	if err != nil {
		return Transition{}, err
	}

	return newTransition(from, to, fromTime, toTime, site, ctxAt)
}

// newTransition is NewTransition with the Context for each instant supplied
// by ctxAt. ctxAt moves one Context, so each position is taken before the
// next instant is asked for.
func newTransition(
	from, to *Block, fromTime, toTime epoch.Time, site *Site, ctxAt func(epoch.Time) *coord.Context,
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
func observedTargetAt(target Observable, t epoch.Time, ctxAt func(epoch.Time) *coord.Context) (coord.AltAz, error) {
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
	Overhead(tr Transition) (unit.Duration, error)
}

// BasicTransitionModel provides a fundamental slew and configuration penalty model.
type BasicTransitionModel struct {
	// BaseSetup is the default overhead applied when initializing pointing
	// if there is no previous block.
	BaseSetup unit.Duration

	// SlewRate is the dome/mount slew speed. A mount's manual quotes it in
	// degrees per second: write [unit.DegreesPerSec] around that number,
	// since a bare one is radians per second.
	SlewRate unit.AngularRate

	// FilterChangePenalty is the time taken to change filters.
	FilterChangePenalty unit.Duration
}

// Overhead calculates the transition time using separation in Alt/Az at the given times.
func (m *BasicTransitionModel) Overhead(tr Transition) (unit.Duration, error) {
	if err := checkFinite("BaseSetup", m.BaseSetup); err != nil {
		return 0, err
	}

	if err := checkFinite("FilterChangePenalty", m.FilterChangePenalty); err != nil {
		return 0, err
	}

	// Initial pointing initialization
	if tr.FromBlock == nil {
		setup := m.BaseSetup
		if setup <= 0 {
			setup = unit.Minutes(1)
		}

		return setup, nil
	}

	var total unit.Duration

	// Configuration overhead
	if tr.FromBlock.Config.Filter != tr.ToBlock.Config.Filter && tr.FromBlock.Config.Filter != "" && tr.ToBlock.Config.Filter != "" {
		total += m.FilterChangePenalty
	}

	// Slew Time. A NaN rate fails the test below, which would drop the slew
	// as though none were configured, so it is refused first.
	if math.IsNaN(float64(m.SlewRate)) {
		return 0, fmt.Errorf("%w: the slew rate is NaN", ErrSlewTime)
	}

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
		slewSeconds := maxAngularDist / m.SlewRate.DegreesPerSec()

		// A slew too slow to finish, a rate so small the time overflows, or a
		// NaN distance, gives a time that is not a number of seconds, and
		// adding it would place the next block at an infinite or NaN instant.
		// Converted to a time.Duration, as it was until unit.Duration replaced
		// it, amd64 and arm64 even disagreed on the result (#1121).
		slew := unit.Seconds(slewSeconds)
		if !finite(float64(slew)) {
			return 0, fmt.Errorf("%w: a %.6g° slew at %.6g deg/s takes %.6g s",
				ErrSlewTime, maxAngularDist, m.SlewRate.DegreesPerSec(), slewSeconds)
		}

		total += slew
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
	// A window that ends before it starts, or at a time that is not a number,
	// is the caller's mistake. Until #1177 it came back as a schedule with
	// every block unscheduled and a nil error, which reads as a night when
	// nothing could be observed.
	if err := checkInterval(window.Start, window.End); err != nil {
		return nil, fmt.Errorf("scheduler: window: %w", err)
	}

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

// defaultStep is a strategy's step when it sets none, a minute.
const defaultStep unit.Duration = 60

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
	start, end epoch.Time,
	step unit.Duration,
	site *Site,
	ctxAt func(epoch.Time) *coord.Context,
	sky *runSky,
	constraints ...Constraint,
) (bool, error) {
	failed, _, err := firstFailure(target, start, end, step, site, ctxAt, sky, constraints...)

	return failed < 0 && err == nil, err
}

// firstFailure is checkConstraintsInterval saying which constraint failed:
// the index in constraints of the first to fail, at the first step where one
// does, with its Result, or -1 when all pass throughout.
func firstFailure(
	target Observable,
	start, end epoch.Time,
	step unit.Duration,
	site *Site,
	ctxAt func(epoch.Time) *coord.Context,
	sky *runSky,
	constraints ...Constraint,
) (int, Result, error) {
	check := func(t epoch.Time) (int, Result, error) {
		ctx := ctxAt(t)

		for i, c := range constraints {
			res, err := checkConstraint(c, target, t, site, ctx, sky)
			if err != nil {
				return -1, Result{}, fmt.Errorf("plan: constraint at %s: %w",
					t.Format(time.RFC3339), err)
			}

			if !res.Pass {
				return i, res, nil
			}
		}

		return -1, Result{}, nil
	}

	t := start
	for t.Before(end) || t.Equal(end) {
		failed, res, err := check(t)
		if err != nil || failed >= 0 {
			return failed, res, err
		}

		t = t.Add(step)
	}

	// Always check the exact end time as well.
	if !start.Equal(end) {
		return check(end)
	}

	return -1, Result{}, nil
}

// GreedyStrategy traverses time forward and schedules the first block in the list
// that satisfies all constraints at the given time. It results in a dense schedule.
type GreedyStrategy struct {
	// Step is the time increment used when searching for a valid start time.
	Step unit.Duration
}

// Schedule implements Strategy for GreedyStrategy.
func (s *GreedyStrategy) Schedule(planner *Planner, window Window, blocks []*Block, transition TransitionModel) (*Schedule, error) {
	ctxAt, err := siteContexts(window.Start, window.End, planner.Site)
	if err != nil {
		return nil, err
	}

	return s.schedule(planner, window, blocks, transition, ctxAt, newRunSky(false))
}

// schedule is Schedule with the Context for each instant supplied by ctxAt,
// which lets a test run the same pass on a full Context per instant.
func (s *GreedyStrategy) schedule(
	planner *Planner,
	window Window,
	blocks []*Block,
	transition TransitionModel,
	ctxAt func(epoch.Time) *coord.Context,
	sky *runSky,
) (*Schedule, error) {
	if err := checkInterval(window.Start, window.End); err != nil {
		return nil, fmt.Errorf("scheduler: window: %w", err)
	}

	if err := checkFinite("Step", s.Step); err != nil {
		return nil, fmt.Errorf("scheduler: %w", err)
	}

	if err := checkBlocks(blocks); err != nil {
		return nil, fmt.Errorf("scheduler: %w", err)
	}

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
		available epoch.Time
		b         *Block
		// constraints are the planner's and the block's, as checked.
		constraints []Constraint
		misses      unscheduledTally
		rem         int
	}

	var unassigned []*activeItem

	for _, b := range blocks {
		repeats := 0
		if b.Cadence != nil {
			repeats = b.Cadence.Repeats
		}

		// Combine base planner constraints with block-specific constraints
		constraints := append(make([]Constraint, 0, len(planner.Constraints)+len(b.Constraints)), planner.Constraints...)
		constraints = append(constraints, b.Constraints...)

		unassigned = append(unassigned, &activeItem{
			b:           b,
			available:   window.Start,
			rem:         repeats,
			constraints: constraints,
			misses:      newUnscheduledTally(len(constraints)),
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
				currentTime, currentTime.Add(overhead), planner.Site, ctxAt)
			if err != nil {
				return nil, fmt.Errorf("plan: greedy: %w", err)
			}

			startTime := currentTime.Add(overhead)
			endTime := startTime.Add(b.Duration)

			if endTime.After(window.End) {
				continue // Block execution exceeds the scheduling window
			}

			// Check observability over the full duration
			failed, res, err := firstFailure(b.Target, startTime, endTime, step, planner.Site, ctxAt, sky, item.constraints...)
			if err != nil {
				return nil, fmt.Errorf("plan: greedy: block %s: %w", b.ID, err)
			}

			item.misses.tried(failed, res)

			if failed < 0 {
				// A greedy run scores each placement once and never returns to
				// it, so it keeps no record of the Moon: one would hold only
				// what is not asked for again, measured at 14% more bytes and
				// no time saved (#959).
				score := scoreBlockPlacement(b, startTime, endTime, planner, ctxAt, sky)
				sched.Blocks = append(sched.Blocks, ScheduledBlock{
					Block:     b,
					Window:    Window{Start: startTime, End: endTime},
					SetupTime: overhead,
					Score:     score,
				})

				currentTime = endTime
				lastBlock = b
				item.misses.observed++

				if item.rem > 0 {
					item.rem--
					// Next observation can start after MinInterval passes
					item.available = endTime.Add(b.Cadence.MinInterval)
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
				currentTime = currentTime.Add(step)
			}
		}
	}

	for _, item := range unassigned {
		sched.Unscheduled = append(sched.Unscheduled, item.misses.unscheduled(item.b, item.constraints))
	}

	return sched, nil
}

// unscheduledTally keeps what a strategy learned about a block it has not
// placed yet: how many placements that fit in the window it tried, how many
// each constraint rejected and, from the last, why, and how many times it
// placed the block.
type unscheduledTally struct {
	rejected []int
	why      []string
	tries    int
	observed int
}

func newUnscheduledTally(constraints int) unscheduledTally {
	return unscheduledTally{rejected: make([]int, constraints), why: make([]string, constraints)}
}

// tried records one placement that fit in the window: failed is the index of
// the constraint that rejected it, with that constraint's result, or -1 when
// none did.
func (t *unscheduledTally) tried(failed int, res Result) {
	t.tries++

	if failed >= 0 {
		t.rejected[failed]++
		t.why[failed] = res.Reason
	}
}

// unscheduled is what the tally says of b, left unscheduled, whose
// constraints are those it was checked against.
func (t *unscheduledTally) unscheduled(b *Block, constraints []Constraint) UnscheduledBlock {
	ub := UnscheduledBlock{Block: b, Observed: t.observed}

	worst := -1

	for i, n := range t.rejected {
		if n > 0 && (worst < 0 || n > t.rejected[worst]) {
			worst = i
		}
	}

	var rejection string

	if worst >= 0 {
		ub.Constraint = constraints[worst]
		rejection = fmt.Sprintf("%T rejected %d of the %d placements tried", ub.Constraint, t.rejected[worst], t.tries)

		if why := t.why[worst]; why != "" {
			rejection += ": " + why
		}
	}

	switch {
	case t.observed > 0:
		ub.Cause = CauseCadence

		times := 1
		if b.Cadence != nil {
			times += b.Cadence.Repeats
		}

		ub.Reason = fmt.Sprintf("observed %d of %d times before the window ended", t.observed, times)
		if rejection != "" {
			ub.Reason += "; " + rejection
		}
	case worst < 0:
		ub.Cause = CauseNoTime
		ub.Reason = "the block and the slew to it never fit in the time the window had left"
	default:
		ub.Cause = CauseConstraint
		ub.Reason = rejection
	}

	return ub
}

// PriorityStrategy pre-sorts blocks by Priority (descending) before applying
// a time-forward greedy allocation, ensuring high-priority blocks win time slots.
type PriorityStrategy struct {
	Step unit.Duration
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
