package plan

import (
	"sort"

	"github.com/TuSKan/astrogo/time"
)

// SwapOptimizedStrategy wraps a base Strategy with local search post-optimization.
//
// After the base strategy produces an initial schedule, iterative passes try
// to improve it:
//  1. Adjacent swap: swap consecutive blocks and accept if total score improves.
//  2. Gap insertion: fit unscheduled blocks into gaps between scheduled blocks.
//
// Each pass is O(n²) where n is the number of scheduled blocks.
// Optimization is monotonic: the total schedule score never decreases.
//
// This is the same local search approach used by production observatory
// schedulers (ESO/VLT, SCHED, STARS).
type SwapOptimizedStrategy struct {
	// Base is the seed strategy that produces the initial schedule.
	// Defaults to PriorityStrategy if nil.
	Base Strategy

	// MaxPasses limits the number of optimization passes. Default: 3.
	// Optimization stops early if a pass produces no improvement.
	MaxPasses int

	// Step is the time increment for constraint validation during
	// swap and insertion feasibility checks. Default: 1 minute.
	Step time.Duration
}

// Schedule produces an optimized schedule by seeding from the base strategy
// and iteratively improving via local search.
func (s *SwapOptimizedStrategy) Schedule(
	planner *Planner,
	window Window,
	blocks []*Block,
	transition TransitionModel,
) (*Schedule, error) {
	base := s.Base
	if base == nil {
		base = &PriorityStrategy{Step: s.Step}
	}

	sched, err := base.Schedule(planner, window, blocks, transition)
	if err != nil {
		return nil, err
	}

	maxPasses := s.MaxPasses
	if maxPasses <= 0 {
		maxPasses = 3
	}
	step := s.Step
	if step <= 0 {
		step = defaultStep
	}

	for pass := 0; pass < maxPasses; pass++ {
		swapped := s.swapPass(sched, planner, transition, step)
		inserted := s.insertPass(sched, planner, window, transition, step)
		if !swapped && !inserted {
			break // converged
		}
	}

	return sched, nil
}

// swapPass tries swapping each pair of adjacent blocks. Accepts a swap if:
//   - Both blocks satisfy constraints at their new times
//   - The swapped block i+1 doesn't collide with block i+2
//   - The combined score improves
//
// Returns true if any swap was accepted.
func (s *SwapOptimizedStrategy) swapPass(
	sched *Schedule,
	planner *Planner,
	transition TransitionModel,
	step time.Duration,
) bool {
	improved := false
	n := len(sched.Blocks)

	for i := 0; i < n-1; i++ {
		bi := sched.Blocks[i]
		bj := sched.Blocks[i+1]

		// Try placing bj at bi's start time.
		newJStart := bi.Window.Start
		newJEnd := newJStart.Add(bj.Block.Duration)

		// Validate bj's constraints at the new time.
		allCJ := mergeConstraints(planner.Constraints, bj.Block.Constraints)
		if !checkConstraintsInterval(bj.Block.Target, newJStart, newJEnd, step, planner.Site, allCJ...) {
			continue
		}

		// Compute transition overhead from bj to bi.
		overhead := time.Duration(0)
		if transition != nil {
			ctx := TransitionContext{
				FromBlock: bj.Block,
				ToBlock:   bi.Block,
				FromTime:  newJEnd,
				ToTime:    newJEnd,
				Site:      planner.Site,
			}
			oh, err := transition.Overhead(ctx)
			if err != nil {
				continue
			}
			overhead = oh
		}

		newIStart := newJEnd.Add(overhead)
		newIEnd := newIStart.Add(bi.Block.Duration)

		// Check collision with the next block after the pair.
		if i+2 < n && newIEnd.After(sched.Blocks[i+2].Window.Start) {
			continue
		}
		// Check window boundary.
		if newIEnd.After(sched.Window.End) {
			continue
		}

		// Validate bi's constraints at the new time.
		allCI := mergeConstraints(planner.Constraints, bi.Block.Constraints)
		if !checkConstraintsInterval(bi.Block.Target, newIStart, newIEnd, step, planner.Site, allCI...) {
			continue
		}

		// Score comparison: accept only if total score improves.
		oldScore := bi.Score + bj.Score
		newJScore := scoreBlockPlacement(bj.Block, newJStart, newJEnd, planner)
		newIScore := scoreBlockPlacement(bi.Block, newIStart, newIEnd, planner)
		newScore := newJScore + newIScore

		if newScore > oldScore {
			sched.Blocks[i] = ScheduledBlock{
				Block:     bj.Block,
				Window:    Window{Start: newJStart, End: newJEnd},
				Score:     newJScore,
				SetupTime: bi.SetupTime,
			}
			sched.Blocks[i+1] = ScheduledBlock{
				Block:     bi.Block,
				Window:    Window{Start: newIStart, End: newIEnd},
				Score:     newIScore,
				SetupTime: overhead,
			}
			improved = true
		}
	}

	return improved
}

// insertPass tries to insert each unscheduled block into a gap in the schedule.
// Blocks are tried in priority order. Each block is placed in the first gap
// where it fits and satisfies constraints.
//
// Returns true if any block was inserted.
func (s *SwapOptimizedStrategy) insertPass(
	sched *Schedule,
	planner *Planner,
	window Window,
	transition TransitionModel,
	step time.Duration,
) bool {
	if len(sched.Unscheduled) == 0 {
		return false
	}

	improved := false
	remaining := make([]UnscheduledBlock, 0, len(sched.Unscheduled))

	// Sort unscheduled by priority (highest first) for best gap allocation.
	sortedUnsched := make([]UnscheduledBlock, len(sched.Unscheduled))
	copy(sortedUnsched, sched.Unscheduled)
	sort.SliceStable(sortedUnsched, func(i, j int) bool {
		return sortedUnsched[i].Block.Priority > sortedUnsched[j].Block.Priority
	})

	for _, ub := range sortedUnsched {
		inserted := false

		gaps := scheduleGaps(sched.Blocks, window)
		for _, gap := range gaps {
			if gap.window.Duration() < ub.Block.Duration {
				continue
			}

			// Compute transition overhead from the preceding block.
			overhead := time.Duration(0)
			if transition != nil {
				ctx := TransitionContext{
					FromBlock: gap.prevBlock,
					ToBlock:   ub.Block,
					FromTime:  gap.window.Start,
					ToTime:    gap.window.Start,
					Site:      planner.Site,
				}
				oh, err := transition.Overhead(ctx)
				if err != nil {
					continue
				}
				overhead = oh
			}

			startTime := gap.window.Start.Add(overhead)
			endTime := startTime.Add(ub.Block.Duration)

			if endTime.After(gap.window.End) {
				continue
			}

			allC := mergeConstraints(planner.Constraints, ub.Block.Constraints)
			if checkConstraintsInterval(ub.Block.Target, startTime, endTime, step, planner.Site, allC...) {
				score := scoreBlockPlacement(ub.Block, startTime, endTime, planner)
				sched.Blocks = append(sched.Blocks, ScheduledBlock{
					Block:     ub.Block,
					Window:    Window{Start: startTime, End: endTime},
					Score:     score,
					SetupTime: overhead,
				})
				inserted = true
				improved = true
				break
			}
		}

		if !inserted {
			remaining = append(remaining, ub)
		}
	}

	sched.Unscheduled = remaining

	// Re-sort blocks chronologically after insertions.
	sort.Slice(sched.Blocks, func(i, j int) bool {
		return sched.Blocks[i].Window.Start.Before(sched.Blocks[j].Window.Start)
	})

	return improved
}

// ── Scheduling Helpers ───────────────────────────────────────────────────────

// mergeConstraints combines two constraint slices into a new slice
// without modifying the originals.
func mergeConstraints(a, b []Constraint) []Constraint {
	merged := make([]Constraint, 0, len(a)+len(b))
	merged = append(merged, a...)
	merged = append(merged, b...)
	return merged
}

// scoreBlockPlacement evaluates how desirable a block placement is by
// scoring the target at the observation midpoint using ScoreObservable.
//
// The score combines altitude (0–90°) weighted by priority, reflecting
// both scientific quality (lower airmass) and user preference.
// Falls back to static block priority if scoring fails.
//
// TODO: Accept a pre-built coord.Context to avoid redundant SOFA matrix
// computation when the caller already has one for the same epoch.
func scoreBlockPlacement(block *Block, start, end time.Time, planner *Planner) float64 {
	mid := start.Add(end.Sub(start) / 2)
	score, err := ScoreObservable(block.Target, mid, planner.Site, planner.Constraints...)
	if err != nil {
		return block.Priority
	}
	return score
}

// gapInfo describes a gap in the schedule.
type gapInfo struct {
	window    Window
	prevBlock *Block // nil if this is the gap before the first block
}

// scheduleGaps computes the gaps between scheduled blocks within a window.
// The returned gaps are ordered chronologically.
func scheduleGaps(blocks []ScheduledBlock, window Window) []gapInfo {
	if len(blocks) == 0 {
		return []gapInfo{{window: window, prevBlock: nil}}
	}

	gaps := make([]gapInfo, 0, len(blocks)+1)

	// Gap before first block.
	if window.Start.Before(blocks[0].Window.Start) {
		gaps = append(gaps, gapInfo{
			window:    Window{Start: window.Start, End: blocks[0].Window.Start},
			prevBlock: nil,
		})
	}

	// Gaps between blocks.
	for i := 0; i < len(blocks)-1; i++ {
		gapStart := blocks[i].Window.End
		gapEnd := blocks[i+1].Window.Start
		if gapStart.Before(gapEnd) {
			gaps = append(gaps, gapInfo{
				window:    Window{Start: gapStart, End: gapEnd},
				prevBlock: blocks[i].Block,
			})
		}
	}

	// Gap after last block.
	lastEnd := blocks[len(blocks)-1].Window.End
	if lastEnd.Before(window.End) {
		gaps = append(gaps, gapInfo{
			window:    Window{Start: lastEnd, End: window.End},
			prevBlock: blocks[len(blocks)-1].Block,
		})
	}

	return gaps
}
