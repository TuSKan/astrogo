package simbad

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/remote"
)

// Provider implements the resolve.Provider and resolve.ObjectResolver
// interfaces interacting with SIMBAD's Table Access Protocol endpoint.
type Provider struct {
	client *remote.Client
	cache  resolve.Cache
}

// New creates a new SIMBAD ObjectResolver.
func New() *Provider {
	client, err := remote.NewClientFor(remote.SIMBAD)
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
	return []resolve.Capability{resolve.CapObjectResolution}
}

// Resolve matches a single object by returning the most relevant hit.
// Adheres strictly to resolve.Provider and utilizes AstroGo scoring for precision.
func (p *Provider) Resolve(query string) (resolve.Target, bool) {
	targets := p.Search(query)
	if len(targets) == 0 {
		return resolve.Target{}, false
	}

	bestIdx := 0
	bestScore := -1.0

	for i, t := range targets {
		s := resolve.Score(query, t.Name)
		if idScore := resolve.Score(query, t.ID); idScore > s {
			s = idScore
		}

		for _, a := range t.Aliases {
			if aScore := resolve.Score(query, a); aScore > s {
				s = aScore
			}
		}

		if s > bestScore {
			bestScore = s
			bestIdx = i
		}
	}

	return targets[bestIdx], true
}

// Search matches all objects closely matching a freeform query.
func (p *Provider) Search(query string) []resolve.Target {
	ctx := context.TODO()
	req := resolve.ObjectRequest{Query: query, Limit: 10}

	iter := p.ResolveObject(ctx, req)

	var targets []resolve.Target

	iter(func(t resolve.Target, err error) bool {
		if err != nil {
			log.Printf("SIMBAD ERR: %v", err)
			return false
		}

		targets = append(targets, t)
		// Try to read up to 10
		return len(targets) < 10
	})

	return targets
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
