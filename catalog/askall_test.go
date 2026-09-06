package catalog

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/time"
)

// barrier holds every provider in Resolve/Search until all of them have
// arrived. Sequential querying therefore cannot satisfy it: the first provider
// waits for a second that will not be called until the first returns.
//
// A barrier rather than a timing comparison, because "concurrent" is a
// structural property and a wall-clock assertion would be a flake waiting for
// a loaded CI machine. The wait is bounded so a sequential resolver reports a
// clean failure instead of deadlocking the package.
type barrier struct {
	arrive   sync.WaitGroup
	release  chan struct{}
	timedOut atomic.Bool
}

// barrierWait is how long one provider will hold before concluding that the
// others are never going to arrive. Never reached when the querying is
// concurrent: the release fires as soon as the last goroutine checks in.
const barrierWait = 2 * time.Second

func (b *barrier) wait() {
	b.arrive.Done()

	select {
	case <-b.release:
	case <-time.After(barrierWait):
		b.timedOut.Store(true)
	}
}

type barrierProvider struct {
	b      *barrier
	name   string
	target Target
}

func (p *barrierProvider) Name() string { return p.name }

func (p *barrierProvider) Resolve(_ context.Context, _ string) (Target, error) {
	p.b.wait()
	return p.target, nil
}

func (p *barrierProvider) Search(_ context.Context, _ string) ([]Target, error) {
	p.b.wait()
	return []Target{p.target}, nil
}

// newBarrier builds n providers that must all be in flight at once.
func newBarrier(t *testing.T, n int) (*barrier, []resolve.Provider) {
	t.Helper()

	b := &barrier{release: make(chan struct{})}
	b.arrive.Add(n)

	providers := make([]resolve.Provider, n)
	for i := range n {
		providers[i] = &barrierProvider{
			b:      b,
			name:   fmt.Sprintf("p%d", i),
			target: Target{ID: fmt.Sprintf("ID%d", i), Name: fmt.Sprintf("N%d", i)},
		}
	}

	go func() {
		b.arrive.Wait()
		close(b.release)
	}()

	return b, providers
}

// assertWasConcurrent fails with the reason rather than with a timeout.
func assertWasConcurrent(t *testing.T, b *barrier) {
	t.Helper()

	if b.timedOut.Load() {
		t.Error("a provider waited out the barrier: the resolver queried them sequentially, " +
			"so each provider's latency adds to the next one's instead of overlapping it")
	}
}

func TestResolveQueriesProvidersConcurrently(t *testing.T) {
	b, providers := newBarrier(t, 4)
	r := &Resolver{providers: providers}

	if _, err := r.Resolve(context.Background(), "N0"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	assertWasConcurrent(t, b)
}

func TestSearchQueriesProvidersConcurrently(t *testing.T) {
	b, providers := newBarrier(t, 4)
	r := &Resolver{providers: providers, cfg: resolverConfig{cap: defaultCap}}

	if _, err := r.Search(context.Background(), "N0"); err != nil {
		t.Fatalf("Search: %v", err)
	}

	assertWasConcurrent(t, b)
}

// TestAskAllPreservesProviderOrder is the guarantee Resolve's "the first group
// is the highest-priority provider's hit" rests on. Replies are written into
// their own slots, so a slow first provider does not lose its place to a fast
// last one.
func TestAskAllPreservesProviderOrder(t *testing.T) {
	// Descending delays: without indexed slots the returned order would be the
	// reverse of the registration order.
	delays := []time.Duration{40 * time.Millisecond, 20 * time.Millisecond, 0}

	providers := make([]resolve.Provider, len(delays))
	for i, d := range delays {
		providers[i] = &slowProvider{name: fmt.Sprintf("p%d", i), delay: d}
	}

	got := askAll(context.Background(), providers, func(ctx context.Context, p Provider) (Target, error) {
		return p.Resolve(ctx, "anything")
	})

	for i, a := range got {
		want := fmt.Sprintf("p%d", i)
		if a.name != want {
			t.Errorf("answers[%d].name = %q, want %q — provider order was lost", i, a.name, want)
		}
	}
}

// TestOneSlowProviderDoesNotCancelTheOthers: askAll is deliberately not
// internal/parallel.Map, whose errgroup cancels the rest on the first error.
// A failing provider must neither cancel nor suppress the ones that would have
// answered.
func TestOneSlowProviderDoesNotCancelTheOthers(t *testing.T) {
	r := &Resolver{providers: []resolve.Provider{
		&mockProvider{name: "broken", failWith: errProviderDown},
		&mockProvider{name: "working", targets: map[string]Target{
			"m42": {ID: "NGC1976", Name: "Orion Nebula"},
		}},
	}}

	got, err := r.Resolve(context.Background(), "M42")
	if err != nil {
		t.Fatalf("a working provider was suppressed by a broken one: %v", err)
	}

	if got.ID != "NGC1976" {
		t.Errorf("Resolve(M42).ID = %q, want NGC1976", got.ID)
	}
}

// errProviderDown stands in for a transport failure — an outage, a rate limit,
// a cancelled context — as opposed to a provider answering "no such object".
var errProviderDown = errors.New("provider is down")

// slowProvider answers after a fixed delay, resolving anything.
type slowProvider struct {
	name  string
	delay time.Duration
}

func (p *slowProvider) Name() string { return p.name }

func (p *slowProvider) Resolve(_ context.Context, _ string) (Target, error) {
	time.Sleep(p.delay)
	return Target{ID: p.name, Name: p.name}, nil
}

func (p *slowProvider) Search(_ context.Context, _ string) ([]Target, error) {
	time.Sleep(p.delay)
	return []Target{{ID: p.name, Name: p.name}}, nil
}
