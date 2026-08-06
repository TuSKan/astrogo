// Package parallel provides a small generic concurrency primitive for the
// "run independent per-item work, collect results in input order" idiom
// this codebase's own errgroup call sites (plan.FilterObservable,
// plan.RankObservable, plan.RankObservables, plan.gatherPlanetaryMoons,
// plan.VisibleTonight's candidate-gathering stages) each hand-rolled
// separately before this package existed.
package parallel

import (
	"runtime"

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
