package simbad

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/remote/api"
)

// Provider implements the resolve.Provider and resolve.ObjectResolver
// interfaces interacting with SIMBAD's Table Access Protocol endpoint.
type Provider struct {
	client *api.Client
	cache  resolve.Cache
}

// New creates a new SIMBAD ObjectResolver.
func New() *Provider {
	client, err := api.NewClient(remote.SIMBAD)
	if err != nil {
		panic(err) // unregistered endpoint would be a programmer error
	}

	return &Provider{
		client: client,
		cache:  resolve.NewMapCache(),
	}
}

// Name returns the provider's display identifier.
func (p *Provider) Name() string {
	return "simbad"
}

// Capabilities lists the supported remote query capacities.
func (p *Provider) Capabilities() []resolve.Capability {
	return []resolve.Capability{resolve.CapObjectResolution, resolve.CapMagnitudeBrowse}
}

// Resolve identifies the single object a name refers to.
//
// # Identity, not the best of a search
//
// This used to be Search's first result after client-side scoring, and the
// scoring could not save it: the underlying query matched any object holding
// the name as a substring of any identifier, so for "M31" the ten rows
// fetched out of 15,843 were Chandra sources inside the galaxy and M31 itself
// was not among them. Scoring ranks what it is given.
//
// It now matches identifiers exactly, against the spellings SIMBAD is known
// to store — see [identifierVariants]. An unknown name returns false rather
// than the least-wrong of ten wrong answers, because a caller can act on
// "not found" and cannot act on a plausible object 70 degrees from the one
// they asked for.
//
// Aliases fan out across rows, so several rows may arrive for one object;
// they are merged by oid upstream in ResolveObject and the first target is
// the object.
func (p *Provider) Resolve(ctx context.Context, query string) (resolve.Target, bool) {
	targets := p.ResolveObjects(ctx, resolve.ObjectRequest{Query: query, Limit: 10})
	if len(targets) == 0 {
		return resolve.Target{}, false
	}

	return targets[0], true
}

// ResolveObjects drains ResolveObject into a slice, dropping errors after
// logging them — the shape both Resolve and Search need.
func (p *Provider) ResolveObjects(ctx context.Context, req resolve.ObjectRequest) []resolve.Target {
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	var targets []resolve.Target

	p.ResolveObject(ctx, req)(func(t resolve.Target, err error) bool {
		if err != nil {
			log.Printf("SIMBAD ERR: %v", err)

			return false
		}

		targets = append(targets, t)

		return len(targets) < limit
	})

	return targets
}

// Search matches objects whose identifiers begin with a freeform query.
//
// Distinct from [Provider.Resolve], and deliberately: several answers are
// this method's purpose, where for Resolve they are a failure. Anchored at
// the start rather than wrapped in wildcards, and ordered brightest-first,
// so the rows are ones a person would recognise and are the same rows on
// every call — the query this replaces had no ORDER BY, and two identical
// searches for "M42" returned different objects.
func (p *Provider) Search(ctx context.Context, query string) []resolve.Target {
	if strings.TrimSpace(query) == "" {
		return nil
	}

	req := resolve.ObjectRequest{Query: query, Limit: 10}
	adql := BuildSearchQuery(req)

	var targets []resolve.Target

	p.stream(ctx, "search:"+query, adql)(func(tgt resolve.Target, err error) bool {
		if err != nil {
			log.Printf("SIMBAD ERR: %v", err)

			return false
		}

		targets = append(targets, tgt)

		return len(targets) < req.Limit
	})

	return targets
}

// SearchBright returns every SIMBAD object brighter than req.MaxVMag,
// brightest-first — the bulk counterpart to Resolve/Search's name-driven
// lookup (see BuildBrightQuery for why this can't reuse ResolveObject's
// query/parse path directly).
func (p *Provider) SearchBright(ctx context.Context, req resolve.BrightRequest) resolve.SeqIterator[resolve.Target] {
	cacheKey := fmt.Sprintf("bright:%f:%d", req.MaxVMag, req.Limit)
	if seq, ok := p.cache.Get(cacheKey); ok {
		return seq
	}

	adql := BuildBrightQuery(req)
	v := TAPRequest(adql)

	return func(yield func(resolve.Target, error) bool) {
		body, err := p.client.PostForm(ctx, remote.SIMBAD, "", v)
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}
		defer func() { _ = body.Close() }()

		data, err := io.ReadAll(body)
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}

		targets, err := ParseBrightCSV(strings.NewReader(string(data)))
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}

		if err := p.cache.Set(cacheKey, targets); err != nil {
			yield(resolve.Target{}, err)
			return
		}

		for _, t := range targets {
			if !yield(t, nil) {
				return
			}
		}
	}
}

// ResolveObject provides an async streaming mechanism using ADQL.
func (p *Provider) ResolveObject(ctx context.Context, req resolve.ObjectRequest) resolve.SeqIterator[resolve.Target] {
	// 1. Check Cache First (Maintain case to prevent ADQL case-sensitive collisions)
	cacheKey := fmt.Sprintf("resolve:%s:%d", req.Query, req.Limit)
	if req.Limit <= 0 {
		cacheKey = fmt.Sprintf("resolve:%s:10", req.Query)
	}

	if seq, ok := p.cache.Get(cacheKey); ok {
		return seq
	}

	adql := BuildResolveQuery(req)
	if adql == "" {
		return func(yield func(resolve.Target, error) bool) {
			yield(resolve.Target{}, ErrEmptyQuery)
		}
	}

	return p.stream(ctx, cacheKey, adql)
}

// stream runs one ADQL query and yields its rows, caching a successful
// fetch. Shared by the identity and search paths, which differ only in the
// query they hand it.
func (p *Provider) stream(ctx context.Context, cacheKey, adql string) resolve.SeqIterator[resolve.Target] {
	v := TAPRequest(adql)

	return func(yield func(resolve.Target, error) bool) {
		body, err := p.client.PostForm(ctx, remote.SIMBAD, "", v)
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}
		defer func() { _ = body.Close() }()

		data, err := io.ReadAll(body)
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}

		targets, err := ParseCSV(strings.NewReader(string(data)))
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}

		// 2. Cache Results on successful fetch
		if err := p.cache.Set(cacheKey, targets); err != nil {
			yield(resolve.Target{}, err)
			return
		}

		for _, t := range targets {
			if !yield(t, nil) {
				return
			}
		}
	}
}
