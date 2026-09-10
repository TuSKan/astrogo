package openngc

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/remote"
)

// Record represents a raw entry in the OpenNGC dataset.
type Record struct {
	ID      string
	Name    string
	Kind    resolve.Kind
	RA      string
	Dec     string
	Aliases []string
}

// Provider implements the resolve.Provider interface for OpenNGC.
type Provider struct {
	// mu guards the load and everything it fills in. It is held across the
	// fetch, so concurrent first queries make one request between them rather
	// than one each.
	mu      sync.Mutex
	remote  *remote.Client
	loaded  bool
	byKey   map[string]int
	targets []resolve.Target
}

// New creates an OpenNGC catalog provider. It performs no I/O.
//
// The catalog — two upstream CSVs, about 7 MB — is fetched on the first query
// that needs it, using that caller's context, and kept for the life of the
// provider.
//
// # Why not at construction
//
// It used to be. New called fetch with context.Background(), which meant a
// constructor that looked pure blocked for as long as the endpoint took: two
// seconds against a warm cache, and a full timeout against an unreachable one,
// with no way for the caller to cancel it or set a deadline. catalog.NewResolver
// takes no context either, so a request-scoped deadline above it had no effect
// on the network call below it. Every other network entry point in astrogo takes
// a ctx first, and this one took none.
//
// A failed load is also no longer permanent. It used to be recorded once and
// replayed at every query for the life of the process, so a consent gate not yet
// granted, or one bad fetch at start-up, made the whole NGC/IC catalog silently
// absent while the provider looked healthy. That is worse than a per-query
// failure precisely because it is invisible and cannot recover; a later query
// now tries again.
func New(opts ...resolve.Option) *Provider {
	return &Provider{remote: resolve.ClientOf(opts)}
}

// Name returns the provider identifier.
func (p *Provider) Name() string { return "openngc" }

// Capabilities returns the set of supported resolution operations.
func (p *Provider) Capabilities() []resolve.Capability {
	return []resolve.Capability{resolve.CapObjectResolution, resolve.CapMagnitudeBrowse}
}

// SearchBright returns every OpenNGC object brighter than req.MaxVMag,
// brightest-first — a plain in-memory filter over the loaded catalog, since
// (unlike SIMBAD/SBDB) there's no remote query to make. ctx carries the
// catalog fetch if this is the first query to need it.
func (p *Provider) SearchBright(ctx context.Context, req resolve.BrightRequest) resolve.SeqIterator[resolve.Target] {
	if err := p.load(ctx); err != nil {
		// The iterator is the only channel this signature has, so the failure
		// goes down it. Yielding nothing instead would report an unreachable
		// catalog as a sky with no bright objects in it.
		return func(yield func(resolve.Target, error) bool) { yield(resolve.Target{}, err) }
	}

	var matches []resolve.Target

	for _, t := range p.targets {
		if t.HasVMag && t.VMag < req.MaxVMag {
			matches = append(matches, t)
		}
	}

	slices.SortFunc(matches, func(a, b resolve.Target) int { return cmp.Compare(a.VMag, b.VMag) })

	if req.Limit > 0 && len(matches) > req.Limit {
		matches = matches[:req.Limit]
	}

	return resolve.SliceSeq(matches)
}

// Resolve performs exact-match resolution for a query.
//
// ctx is used: the first query to reach a provider fetches the catalog under
// it. After that the lookup is an in-memory index and there is nothing left to
// cancel.
func (p *Provider) Resolve(ctx context.Context, query string) (resolve.Target, error) {
	if err := p.load(ctx); err != nil {
		return resolve.Target{}, err
	}

	q := resolve.Normalize(query)
	if idx, ok := p.byKey[q]; ok {
		return p.targets[idx], nil
	}

	return resolve.Target{}, fmt.Errorf("%w: %q in OpenNGC", resolve.ErrNotFound, query)
}

// Search performs fuzzy search across all NGC/IC objects. ctx carries the
// first query's catalog fetch — see Resolve.
func (p *Provider) Search(ctx context.Context, query string) ([]resolve.Target, error) {
	if err := p.load(ctx); err != nil {
		return nil, err
	}

	q := resolve.Normalize(query)
	if q == "" {
		return nil, nil
	}

	var results []resolve.Target

	for _, t := range p.targets {
		if strings.Contains(resolve.Normalize(t.Name), q) ||
			strings.Contains(resolve.Normalize(t.ID), q) {
			results = append(results, t)
			continue
		}

		for _, a := range t.Aliases {
			if strings.Contains(resolve.Normalize(a), q) {
				results = append(results, t)
				break
			}
		}
	}

	return results, nil
}

// load fetches and indexes the catalog once, under the caller's context.
//
// A failure is deliberately not cached: the next query retries. The cost of
// that is a second attempt against an endpoint that is still down; the cost of
// caching it is a provider that answers "not found" for ever because of one
// cancelled context at start-up.
func (p *Provider) load(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.loaded {
		return nil
	}

	targets, err := fetch(ctx, p.client())
	if err != nil {
		return fmt.Errorf("openngc: catalog unavailable: %w", err)
	}

	byKey := make(map[string]int, len(targets)*2)

	for i, t := range targets {
		byKey[resolve.Normalize(t.ID)] = i
		if t.Name != "" {
			byKey[resolve.Normalize(t.Name)] = i
		}

		for _, a := range t.Aliases {
			byKey[resolve.Normalize(a)] = i
		}
	}

	p.targets, p.byKey, p.loaded = targets, byKey, true

	return nil
}

// client is the policy this provider fetches under: its own if [WithClient]
// gave it one, otherwise the process default. Resolved per call rather than at
// construction so a provider built before [remote.SetDataDir] still sees it.
func (p *Provider) client() *remote.Client {
	if p.remote == nil {
		return remote.Default()
	}

	return p.remote
}
