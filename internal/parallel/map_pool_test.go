package parallel

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Map runs on a pool of workers that each take the next index, not on a
// goroutine per item (#955). These tests hold what that must not change: every
// item runs, the bound is the bound, and the dispatch is dynamic.

// TestMap_EveryItemRunsAfterAnError: an error is returned once every call has
// finished, and no item is skipped because an earlier one failed, as with
// errgroup.Group without a context. Callers such as VisibleTonight record each
// item's own outcome and rely on all of them running. One worker, so no
// other worker can finish the items one that stopped early left.
func TestMap_EveryItemRunsAfterAnError(t *testing.T) {
	const n = 50

	in := make([]int, n)

	var calls atomic.Int32

	_, err := Map(in, 1, func(i int, _ int) (int, error) {
		calls.Add(1)

		if i == 0 {
			return 0, errMapTest
		}

		return i, nil
	})
	if !errors.Is(err, errMapTest) {
		t.Fatalf("Map error = %v, want errMapTest", err)
	}

	if got := calls.Load(); got != n {
		t.Errorf("f was called %d times, want %d: an error must not skip the items after it", got, n)
	}
}

// TestMap_RunsLimitCallsAtOnce: the pool really has limit workers. The first
// limit items wait until limit of them are running at once, which they can
// only do if that many workers exist, and no call ever sees more than limit.
func TestMap_RunsLimitCallsAtOnce(t *testing.T) {
	const n, limit = 40, 4

	in := make([]int, n)

	var (
		active, peak atomic.Int32
		arrived      sync.WaitGroup
		release      = make(chan struct{})
	)

	arrived.Add(limit)

	go func() {
		arrived.Wait()
		close(release)
	}()

	_, err := Map(in, limit, func(i int, _ int) (int, error) {
		now := active.Add(1)
		defer active.Add(-1)

		for p := peak.Load(); now > p && !peak.CompareAndSwap(p, now); p = peak.Load() {
		}

		if i < limit {
			arrived.Done()

			select {
			case <-release:
			case <-time.After(10 * time.Second):
				return 0, errors.New("fewer than limit calls ran at once")
			}
		}

		return i, nil
	})
	if err != nil {
		t.Fatalf("Map: %v", err)
	}

	if got := peak.Load(); got != limit {
		t.Errorf("peak concurrent calls = %d, want exactly %d", got, limit)
	}
}

// TestMap_ABlockedItemDoesNotHoldBackTheRest: dispatch is dynamic, which is
// what keeps uneven work balanced. Item 0 blocks until every other item has
// finished. Fixed chunks would leave the items queued behind it waiting
// forever; here the other worker takes them.
func TestMap_ABlockedItemDoesNotHoldBackTheRest(t *testing.T) {
	const n = 20

	in := make([]int, n)

	var others sync.WaitGroup

	others.Add(n - 1)

	_, err := Map(in, 2, func(i int, _ int) (int, error) {
		if i != 0 {
			others.Done()

			return i, nil
		}

		done := make(chan struct{})

		go func() {
			others.Wait()
			close(done)
		}()

		select {
		case <-done:
			return 0, nil
		case <-time.After(10 * time.Second):
			return 0, errors.New("the other items waited behind a blocked one")
		}
	})
	if err != nil {
		t.Fatalf("Map: %v", err)
	}
}

// deepStack recurses with a frame large enough that a fresh goroutine must
// grow its stack several times to get to the bottom, as plan.Scorer.Score does
// on its way into SOFA.
//
//go:noinline
func deepStack(depth int) int {
	var frame [512]byte

	frame[depth%len(frame)] = byte(depth)

	if depth == 0 {
		return int(frame[0])
	}

	return deepStack(depth-1) + int(frame[depth%len(frame)])
}

// BenchmarkMapDeepStack is Map over items that each need a deep stack, the
// cost #955 measured as a fifth of a ranking: a goroutine per item grew its
// stack afresh for every item, and a worker grows it once.
func BenchmarkMapDeepStack(b *testing.B) {
	in := make([]int, 1000)

	b.ReportAllocs()

	for b.Loop() {
		if _, err := Map(in, 0, func(i int, _ int) (int, error) { return deepStack(64), nil }); err != nil {
			b.Fatal(err)
		}
	}
}
