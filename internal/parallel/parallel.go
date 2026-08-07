// Package parallel provides small generic concurrency primitives for two
// distinct "run independent work across goroutines" shapes this codebase
// otherwise hand-rolls per call site:
//
//   - Map: one goroutine call per item, bounded concurrency — right for
//     uneven or expensive per-item work (a network fetch, an ephemeris
//     lookup). Used by plan.FilterObservable, plan.RankObservable,
//     plan.RankObservables, plan.gatherPlanetaryMoons, and
//     plan.VisibleTonight's candidate-gathering stages, each of which
//     hand-rolled its own errgroup before this package existed.
//   - MapChunked: a fixed number of goroutines, each processing a
//     contiguous chunk of indices — right for cheap, uniform per-item work
//     where goroutine-scoped setup (e.g. cloning a not-safe-to-share
//     resource) must happen once per goroutine, not once per item. Used by
//     coord.Context.ReduceBatchParallel/ICRSBatchToAltAzParallel, which
//     hand-rolled the identical chunk-and-clone loop twice before this
//     package existed.
package parallel

import (
	"runtime"
	"sync"

	"golang.org/x/sync/errgroup"
)

// Map applies f to each element of in concurrently, bounded by limit
// concurrent calls at a time, and returns the results in the same order
// as in — not the order calls happen to finish. limit <= 0 means
// runtime.GOMAXPROCS(0) — the right default for pure CPU/in-memory work
// with no external resource to be considerate of; pass a smaller explicit
// limit for work that hits a shared external service (e.g. a handful of
// concurrent network requests to the same API).
//
// If any call returns a non-nil error, Map returns that error (the first
// one errgroup.Group observes; the others' results are discarded) once
// every in-flight call has finished — matching errgroup.Group's own
// first-error-wins semantics, not a hard cancellation of work already in
// flight.
func Map[T, R any](in []T, limit int, f func(i int, item T) (R, error)) ([]R, error) {
	if limit <= 0 {
		limit = runtime.GOMAXPROCS(0)
	}

	out := make([]R, len(in))

	g := new(errgroup.Group)
	g.SetLimit(limit)

	for i, item := range in {
		g.Go(func() error {
			r, err := f(i, item)
			if err != nil {
				return err
			}

			out[i] = r

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err //nolint:wrapcheck // transparent passthrough of f's own error is the point — wrapping here would break a caller's errors.Is against f's sentinel
	}

	return out, nil
}

// MapChunked runs f over every index in [0, n) using workers goroutines,
// each handling one contiguous chunk of indices — the shape Map does not
// cover: goroutine-scoped setup that must happen once per goroutine, not
// once per item.
//
// newWorker is called exactly once per goroutine (workers times, not n
// times — or once total on the small-n synchronous path below) to build
// whatever state that goroutine's f calls need; return the same value
// every time if no per-goroutine setup is required. f is responsible for
// writing its own result (typically indexing into a caller-owned output
// slice from a closure, e.g. out[i] = ...) — MapChunked itself returns
// nothing, matching the write-into-caller-supplied-slice convention its
// two call sites (coord.Context.ReduceBatchParallel/ICRSBatchToAltAzParallel)
// already use.
//
// workers <= 0 means runtime.GOMAXPROCS(0). For n < 2*workers, MapChunked
// runs synchronously in the calling goroutine with a single newWorker
// call and no goroutines spawned at all — chunking a batch too small to
// benefit would only add scheduling overhead.
func MapChunked[W any](n, workers int, newWorker func() W, f func(w W, i int)) {
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}

	if n < workers*2 {
		w := newWorker()
		for i := range n {
			f(w, i)
		}

		return
	}

	var wg sync.WaitGroup

	chunkSize := (n + workers - 1) / workers

	for start := 0; start < n; start += chunkSize {
		end := min(start+chunkSize, n)

		wg.Add(1)

		go func(lo, hi int) {
			defer wg.Done()

			w := newWorker()
			for i := lo; i < hi; i++ {
				f(w, i)
			}
		}(start, end)
	}

	wg.Wait()
}
