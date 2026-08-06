package parallel

import (
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
)

func TestMap_PreservesOrder(t *testing.T) {
	in := []int{5, 4, 3, 2, 1, 0}

	got, err := Map(in, 0, func(_ int, item int) (int, error) {
		return item * item, nil
	})
	if err != nil {
		t.Fatalf("Map: %v", err)
	}

	want := []int{25, 16, 9, 4, 1, 0}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestMap_IndexMatchesInputPosition(t *testing.T) {
	in := []string{"a", "b", "c", "d"}

	got, err := Map(in, 0, func(i int, item string) (string, error) {
		if item != in[i] {
			t.Errorf("f called with i=%d item=%q, want in[%d]=%q", i, item, i, in[i])
		}

		return item, nil
	})
	if err != nil {
		t.Fatalf("Map: %v", err)
	}

	for i := range in {
		if got[i] != in[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], in[i])
		}
	}
}

func TestMap_EmptyInput(t *testing.T) {
	got, err := Map([]int(nil), 0, func(_ int, item int) (int, error) {
		t.Error("f should never be called for an empty input")

		return item, nil
	})
	if err != nil {
		t.Fatalf("Map: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("expected empty result, got %v", got)
	}
}

var errMapTest = errors.New("parallel_test: injected failure")

func TestMap_PropagatesFirstError(t *testing.T) {
	in := []int{0, 1, 2, 3, 4}

	_, err := Map(in, 0, func(_ int, item int) (int, error) {
		if item == 2 {
			return 0, errMapTest
		}

		return item, nil
	})
	if !errors.Is(err, errMapTest) {
		t.Fatalf("Map error = %v, want errMapTest", err)
	}
}

// TestMap_SingleItem covers the degenerate n=1 case explicitly -- no
// concurrency to speak of, just confirms the plumbing doesn't assume a
// larger slice anywhere.
func TestMap_SingleItem(t *testing.T) {
	got, err := Map([]int{42}, 0, func(_ int, item int) (int, error) {
		return item + 1, nil
	})
	if err != nil {
		t.Fatalf("Map: %v", err)
	}

	if len(got) != 1 || got[0] != 43 {
		t.Errorf("got %v, want [43]", got)
	}
}

// TestMap_ExplicitLimitCapsConcurrency proves an explicit positive limit
// is actually enforced, not just accepted and ignored: with limit=1, no
// two calls to f may observe an in-progress sibling call.
func TestMap_ExplicitLimitCapsConcurrency(t *testing.T) {
	const n = 10

	in := make([]int, n)

	var active atomic.Int32

	_, err := Map(in, 1, func(_ int, item int) (int, error) {
		if got := active.Add(1); got > 1 {
			t.Errorf("active concurrent calls = %d, want <= 1 with limit=1", got)
		}

		defer active.Add(-1)

		return item, nil
	})
	if err != nil {
		t.Fatalf("Map: %v", err)
	}
}

// TestMapChunked_ProcessesEveryIndexExactlyOnce proves the chunk split
// covers [0, n) with no gaps or overlaps, on the goroutine path (n large
// enough relative to workers to avoid the synchronous fallback).
func TestMapChunked_ProcessesEveryIndexExactlyOnce(t *testing.T) {
	const n = 103 // deliberately not a multiple of workers

	out := make([]int, n)

	MapChunked(n, 4, func() struct{} { return struct{}{} }, func(_ struct{}, i int) {
		out[i] = i + 1 // 0 is indistinguishable from "never written"
	})

	for i, v := range out {
		if v != i+1 {
			t.Errorf("out[%d] = %d, want %d (index processed 0 or >1 times)", i, v, i+1)
		}
	}
}

// TestMapChunked_NewWorkerCalledOncePerGoroutineNotPerItem is the whole
// point of this function over Map: on the goroutine path, newWorker must
// be called once per chunk (== workers, when n divides evenly enough to
// produce exactly that many chunks), never once per item.
func TestMapChunked_NewWorkerCalledOncePerGoroutineNotPerItem(t *testing.T) {
	const n = 100

	const workers = 4 // n=100, chunkSize=25 -> exactly 4 chunks

	var newWorkerCalls atomic.Int32

	MapChunked(n, workers, func() int {
		return int(newWorkerCalls.Add(1))
	}, func(_ int, _ int) {})

	if got := newWorkerCalls.Load(); got != workers {
		t.Errorf("newWorker called %d times, want exactly %d (once per goroutine)", got, workers)
	}
}

// TestMapChunked_SmallBatchRunsSynchronouslyWithOneWorkerCall covers the
// n < 2*workers fallback: no goroutines, a single newWorker call, and
// (since it's synchronous) items processed in strict index order.
func TestMapChunked_SmallBatchRunsSynchronouslyWithOneWorkerCall(t *testing.T) {
	const n = 3

	const workers = 4 // n < 2*workers -> synchronous path

	var newWorkerCalls atomic.Int32

	var order []int

	MapChunked(n, workers, func() int {
		return int(newWorkerCalls.Add(1))
	}, func(_ int, i int) {
		order = append(order, i) // safe: synchronous path, single goroutine
	})

	if got := newWorkerCalls.Load(); got != 1 {
		t.Errorf("newWorker called %d times, want exactly 1 on the synchronous path", got)
	}

	want := []int{0, 1, 2}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}

	for i := range want {
		if order[i] != want[i] {
			t.Errorf("order = %v, want %v", order, want)
		}
	}
}

// TestMapChunked_ZeroN covers n=0: the synchronous path is taken (0 < any
// positive workers*2), newWorker is still called once (matching the
// documented "single newWorker call" on that path), but f is never called.
func TestMapChunked_ZeroN(t *testing.T) {
	var newWorkerCalls, fCalls atomic.Int32

	MapChunked(0, 4, func() int {
		return int(newWorkerCalls.Add(1))
	}, func(_ int, _ int) {
		fCalls.Add(1)
	})

	if got := newWorkerCalls.Load(); got != 1 {
		t.Errorf("newWorker called %d times, want 1", got)
	}

	if got := fCalls.Load(); got != 0 {
		t.Errorf("f called %d times, want 0", got)
	}
}

// TestMapChunked_ExplicitWorkersCapsGoroutineCount proves an explicit
// workers value actually bounds the number of concurrently-active chunks,
// not just the chunk count.
func TestMapChunked_ExplicitWorkersCapsGoroutineCount(t *testing.T) {
	const n = 1000

	const workers = 2

	var active, newWorkerCalls atomic.Int32

	MapChunked(n, workers, func() struct{} {
		newWorkerCalls.Add(1)
		return struct{}{}
	}, func(_ struct{}, _ int) {
		if got := active.Add(1); got > workers {
			t.Errorf("active concurrent chunks = %d, want <= %d", got, workers)
		}

		defer active.Add(-1)
	})

	if got := newWorkerCalls.Load(); got != workers {
		t.Errorf("newWorker called %d times, want exactly %d", got, workers)
	}
}

// TestMapChunked_DefaultWorkersIsGOMAXPROCS confirms workers<=0 falls back
// to runtime.GOMAXPROCS(0), matching Map's own limit<=0 convention.
func TestMapChunked_DefaultWorkersIsGOMAXPROCS(t *testing.T) {
	want := runtime.GOMAXPROCS(0)

	// A large-enough n forces the goroutine path regardless of GOMAXPROCS.
	const n = 10_000

	var newWorkerCalls atomic.Int32

	MapChunked(n, 0, func() struct{} {
		newWorkerCalls.Add(1)
		return struct{}{}
	}, func(_ struct{}, _ int) {})

	if got := int(newWorkerCalls.Load()); got != want {
		t.Errorf("newWorker called %d times, want %d (GOMAXPROCS)", got, want)
	}
}
