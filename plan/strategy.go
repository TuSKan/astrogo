package plan

import (
	"sort"

	"github.com/TuSKan/astrogo/coord"
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
// IMPORTANT: This is a local search heuristic, not a global optimizer.
// It does not guarantee the globally optimal schedule. This is the same
// trade-off made by production observatory schedulers (ESO/VLT, SCHED, STARS)
// where tractability and monotonic improvement are preferred over the
// NP-hard combinatorial global optimum. Users requiring global optimization
// (e.g., integer programming, branch-and-bound) should implement the
// Strategy interface directly.
type SwapOptimizedStrategy struct {
	// Base is the seed strategy that produces the initial schedule.
	// Defaults to PriorityStrategy if nil.
	Base Strategy

	// MaxPasses limits the number of optimization passes. Default: 20.
	// Optimization stops early if two consecutive passes produce no change.
	MaxPasses int

	// Step is the time increment for constraint validation during
	// swap and insertion feasibility checks. Default: 1 minute.
	Step time.Duration

	// TabuTenure is the number of passes a swap pair stays tabu.
	// Default: 3. Set to 0 to disable tabu search.
	TabuTenure int
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
		maxPasses = 20
	}
	step := s.Step
	if step <= 0 {
		step = defaultStep
	}

	// Initialize tabu list for swap-cycle prevention.
	tenure := s.TabuTenure
	if tenure <= 0 {
		tenure = 3
	}
	tabu := newTabuList(tenure)

	// Iterate until convergence: two consecutive passes with no improvement,
	// or maxPasses reached as a safety cap.
	idlePasses := 0
	for pass := 0; pass < maxPasses; pass++ {
		swapped := s.swapPass(sched, planner, transition, step, tabu, pass)
		inserted := s.insertPass(sched, planner, window, transition, step)
		if swapped || inserted {
			idlePasses = 0
		} else {
			idlePasses++
			if idlePasses >= 2 {
				break // converged: two passes with zero improvement
			}
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
	tabu *tabuList,
	passNum int,
) bool {
	improved := false
	n := len(sched.Blocks)

	// Pre-compute merged constraints per block to avoid re-allocating on every candidate.
	mergedC := make(map[string][]Constraint, n)
	for _, sb := range sched.Blocks {
		if _, ok := mergedC[sb.Block.ID]; !ok {
			mergedC[sb.Block.ID] = mergeConstraints(planner.Constraints, sb.Block.Constraints)
		}
	}

	for i := 0; i < n-1; i++ {
		bi := sched.Blocks[i]
		bj := sched.Blocks[i+1]

		// Try placing bj at bi's start time.
		newJStart := bi.Window.Start
		newJEnd := newJStart.Add(bj.Block.Duration)

		// Validate bj's constraints at the new time.
		midCtxJ, okJ := checkConstraintsIntervalCtx(bj.Block.Target, newJStart, newJEnd, step, planner.Site, mergedC[bj.Block.ID]...)
		if !okJ {
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
		midCtxI, okI := checkConstraintsIntervalCtx(bi.Block.Target, newIStart, newIEnd, step, planner.Site, mergedC[bi.Block.ID]...)
		if !okI {
			continue
		}

		// Score comparison: accept if total score improves or equals (plateau move).
		// Plateau moves (>=) allow escape from local maxima; oscillation is bounded
		// by the convergence check in Schedule() which stops after two idle passes.
		oldScore := bi.Score + bj.Score
		newJScore := scoreBlockPlacement(bj.Block, newJStart, newJEnd, planner, midCtxJ)
		newIScore := scoreBlockPlacement(bi.Block, newIStart, newIEnd, planner, midCtxI)
		newScore := newJScore + newIScore

		if newScore >= oldScore {
			// Check tabu status: skip if this pair was recently swapped.
			if tabu.isTabu(bi.Block.ID, bj.Block.ID, passNum) {
				continue
			}

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

			// Record the reverse swap as tabu.
			tabu.add(bi.Block.ID, bj.Block.ID, passNum)
		}
	}

	return improved
}

// insertPass tries to insert each unscheduled block into a gap in the schedule.
// Blocks are tried in priority order. Each block is placed in the first gap
// where it fits and satisfies constraints.
// Gaps are computed once per pass (not per candidate) and the schedule is
// re-sorted after all insertions.
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

	// Pre-compute merged constraints per unscheduled block.
	mergedC := make(map[string][]Constraint, len(sortedUnsched))
	for _, ub := range sortedUnsched {
		if _, ok := mergedC[ub.Block.ID]; !ok {
			mergedC[ub.Block.ID] = mergeConstraints(planner.Constraints, ub.Block.Constraints)
		}
	}

	// Compute gaps once before iterating over unscheduled blocks.
	gaps := scheduleGaps(sched.Blocks, window)

	for _, ub := range sortedUnsched {
		inserted := false

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

			if midCtx, ok := checkConstraintsIntervalCtx(ub.Block.Target, startTime, endTime, step, planner.Site, mergedC[ub.Block.ID]...); ok {
				score := scoreBlockPlacement(ub.Block, startTime, endTime, planner, midCtx)
				sched.Blocks = append(sched.Blocks, ScheduledBlock{
					Block:     ub.Block,
					Window:    Window{Start: startTime, End: endTime},
					Score:     score,
					SetupTime: overhead,
				})
				inserted = true
				improved = true

				// Rebuild gaps to account for the insertion.
				sort.Slice(sched.Blocks, func(i, j int) bool {
					return sched.Blocks[i].Window.Start.Before(sched.Blocks[j].Window.Start)
				})
				gaps = scheduleGaps(sched.Blocks, window)
				break
			}
		}

		if !inserted {
			remaining = append(remaining, ub)
		}
	}

	sched.Unscheduled = remaining

	// Final chronological sort.
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
// If ctx is non-nil it is reused; otherwise ScoreObservable creates one.
// Falls back to static block priority if scoring fails.
func scoreBlockPlacement(block *Block, start, end time.Time, planner *Planner, ctx *coord.Context) float64 {
	mid := start.Add(end.Sub(start) / 2)
	score, err := ScoreObservable(block.Target, mid, planner.Site, nil, ctx, planner.Constraints...)
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

// ── Tabu Search ──────────────────────────────────────────────────────────────

// tabuKey identifies a pair of blocks that were recently swapped.
type tabuKey struct{ a, b string }

// tabuList prevents swap oscillation by tracking recently swapped block pairs.
// A swap pair remains tabu for `tenure` passes after being recorded.
type tabuList struct {
	set    map[tabuKey]int // value = pass number when added
	tenure int
}

func newTabuList(tenure int) *tabuList {
	return &tabuList{
		set:    make(map[tabuKey]int),
		tenure: tenure,
	}
}

// isTabu returns true if swapping (a, b) is currently forbidden.
func (t *tabuList) isTabu(a, b string, currentPass int) bool {
	if t.tenure == 0 {
		return false
	}
	// Check both orderings
	if pass, ok := t.set[tabuKey{a, b}]; ok && currentPass-pass < t.tenure {
		return true
	}
	if pass, ok := t.set[tabuKey{b, a}]; ok && currentPass-pass < t.tenure {
		return true
	}
	return false
}

// add records that blocks (a, b) were swapped at passNum.
func (t *tabuList) add(a, b string, passNum int) {
	t.set[tabuKey{a, b}] = passNum
}
