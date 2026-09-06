package catalog

import (
	"context"
	"sync"
)

// answer is one provider's reply to one query, kept whether it succeeded or
// failed. Both are load-bearing: a success is a candidate to reconcile, and a
// failure is the difference between "no such object" and "nobody could say"
// — see [Resolver.Resolve].
type answer[R any] struct {
	name string
	val  R
	err  error
}

// askAll puts one query to every provider at once and returns their replies in
// provider-registration order.
//
// # Why concurrent
//
// This used to be a plain loop, so resolving a name cost the sum of every
// provider's latency rather than the slowest one's. A resolver over SIMBAD and
// OpenNGC paid a CDS round-trip on top of a local lookup for every query, and a
// pipeline resolving ten thousand names paid it ten thousand times. See #138.
//
// Concurrency across providers is not concurrency across queries: each service
// receives exactly the one request it would have received anyway, at the same
// rate, so this adds nothing to anyone's load. Being polite to other people's
// services is this project's stated policy and it is untouched here.
//
// # Why not internal/parallel.Map
//
// Map is an errgroup: the first error wins and the remaining work is
// cancelled. That is the opposite of what a resolver needs. A provider being
// unreachable must not cancel the ones that would have answered, and its
// failure has to survive to be reported alongside the successes rather than
// replacing them.
//
// # Why order is preserved
//
// Resolver.Resolve returns the first reconciled group, which is the
// highest-priority provider's hit unless it merged with a later one. Writing
// each reply into its own slot rather than appending as replies arrive keeps
// that meaning exactly, so provider order stays a caller-visible guarantee
// rather than a scheduling accident.
func askAll[R any](ctx context.Context, providers []Provider, ask func(context.Context, Provider) (R, error)) []answer[R] {
	answers := make([]answer[R], len(providers))

	var wg sync.WaitGroup

	for i, p := range providers {
		wg.Go(func() {
			val, err := ask(ctx, p)
			answers[i] = answer[R]{name: p.Name(), val: val, err: err}
		})
	}

	wg.Wait()

	return answers
}
