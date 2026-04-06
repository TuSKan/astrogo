package plan

import (
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
	ID          string
	Target      Observable
	Duration    time.Duration
	Config      Configuration
	Priority    float64
	Constraints []Constraint
	Cadence     *Cadence
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

// TransitionContext contains the information needed to evaluate transition overhead.
type TransitionContext struct {
	FromBlock *Block // Can be nil if this is the first block
	ToBlock   *Block
	FromTime  time.Time // Time when the previous observation ended
	ToTime    time.Time // Time when the next observation begins (approximate, often FromTime)
	Site      *Site
}

// TransitionModel evaluates the overhead of moving between two observations.
type TransitionModel interface {
	Overhead(ctx TransitionContext) (time.Duration, error)
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
func (m *BasicTransitionModel) Overhead(ctx TransitionContext) (time.Duration, error) {
	// Initial pointing initialization
	if ctx.FromBlock == nil {
		setup := m.BaseSetup
		if setup <= 0 {
			setup = 1 * time.Minute
		}
		return setup, nil
	}

	var total time.Duration

	// Configuration overhead
	if ctx.FromBlock.Config.Filter != ctx.ToBlock.Config.Filter && ctx.FromBlock.Config.Filter != "" && ctx.ToBlock.Config.Filter != "" {
		total += m.FilterChangePenalty
	}

	// Slew Time
	if m.SlewRate > 0 {
		posFrom, err := ctx.FromBlock.Target.Position(ctx.FromTime)
		if err != nil {
			return 0, err
		}

		posTo, err := ctx.ToBlock.Target.Position(ctx.ToTime)
		if err != nil {
			return 0, err
		}

		altAzFrom, err := coord.ICRSToAltAz(posFrom, ctx.FromTime, ctx.Site.Location())
		if err != nil {
			return 0, err
		}

		altAzTo, err := coord.ICRSToAltAz(posTo, ctx.ToTime, ctx.Site.Location())
		if err != nil {
			return 0, err
		}

		// Calculate separation on Alt and Az independently.
		// Assuming simultaneous slew on two independent axes, slew time is
		// determined by the axis that takes the longest.
		dAlt := math.Abs(altAzFrom.Alt().Degrees() - altAzTo.Alt().Degrees())

		azFrom := altAzFrom.Az().Degrees()
		azTo := altAzTo.Az().Degrees()
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

	return s.Strategy.Schedule(s.Planner, window, blocks, s.TransitionModel)
}

const defaultStep = 1 * time.Minute

// checkConstraintsInterval verifies that all constraints pass continuously over a time range.
func checkConstraintsInterval(target Observable, start, end time.Time, step time.Duration, site *Site, constraints ...Constraint) bool {
	t := start
	for t.Before(end) || t.Equal(end) {
		eval, err := IsObservable(target, t, site, constraints...)
		if err != nil || !eval.Observable {
			return false
		}
		t = t.Add(step)
	}

	// Always check the exact end time as well.
	if !start.Equal(end) {
		eval, err := IsObservable(target, end, site, constraints...)
		if err != nil || !eval.Observable {
			return false
		}
	}

	return true
}

// GreedyStrategy traverses time forward and schedules the first block in the list
// that satisfies all constraints at the given time. It results in a dense schedule.
type GreedyStrategy struct {
	// Step is the time increment used when searching for a valid start time.
	Step time.Duration
}

// Schedule implements Strategy for GreedyStrategy.
func (s *GreedyStrategy) Schedule(planner *Planner, window Window, blocks []*Block, transition TransitionModel) (*Schedule, error) {
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

			// Calculate transition overhead
			ctx := TransitionContext{
				FromBlock: lastBlock,
				ToBlock:   b,
				FromTime:  currentTime,
				ToTime:    currentTime, // Initial approximation
				Site:      planner.Site,
			}
			overhead, err := transition.Overhead(ctx)
			if err != nil {
				continue
			}

			// Refine Transition Overhead with better approximation of destination time
			ctx.ToTime = currentTime.Add(overhead)
			overhead, _ = transition.Overhead(ctx)

			startTime := currentTime.Add(overhead)
			endTime := startTime.Add(b.Duration)

			if endTime.After(window.End) {
				continue // Block execution exceeds the scheduling window
			}

			// Combine base planner constraints with block-specific constraints
			allConstraints := append(make([]Constraint, 0, len(planner.Constraints)+len(b.Constraints)), planner.Constraints...)
			allConstraints = append(allConstraints, b.Constraints...)

			// Check observability over the full duration
			if checkConstraintsInterval(b.Target, startTime, endTime, step, planner.Site, allConstraints...) {
				sched.Blocks = append(sched.Blocks, ScheduledBlock{
					Block:     b,
					Window:    Window{Start: startTime, End: endTime},
					SetupTime: overhead,
					Score:     b.Priority, // Simple score default
				})

				currentTime = endTime
				lastBlock = b

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
