package simbad

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/TuSKan/astrogo/catalog/provider"
)

// Provider implements the provider.Provider and provider.ObjectResolver
// interfaces interacting with SIMBAD's Table Access Protocol endpoint.
type Provider struct {
	client *provider.Client
	cache  provider.Cache
}

// New creates a new SIMBAD ObjectResolver.
func New() *Provider {
	return &Provider{
		client: provider.NewClient(),
		cache:  provider.NewArrowCache(),
	}
}

// Name returns the provider's display identifier.
func (p *Provider) Name() string {
	return "simbad"
}

// Capabilities lists the supported remote query capacities.
func (p *Provider) Capabilities() []provider.Capability {
	return []provider.Capability{provider.CapObjectResolution}
}

// Resolve matches a single object by returning the most relevant hit.
// Adheres strictly to provider.Provider and utilizes AstroGo scoring for precision.
func (p *Provider) Resolve(query string) (provider.Target, bool) {
	targets := p.Search(query)
	if len(targets) == 0 {
		return provider.Target{}, false
	}

	bestIdx := 0
	bestScore := -1.0
	for i, t := range targets {
		s := provider.Score(query, t.Name)
		if idScore := provider.Score(query, t.ID); idScore > s {
			s = idScore
		}
		for _, a := range t.Aliases {
			if aScore := provider.Score(query, a); aScore > s {
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
func (p *Provider) Search(query string) []provider.Target {
	ctx := context.TODO()
	req := provider.ObjectRequest{Query: query, Limit: 10}

	iter := p.ResolveObject(ctx, req)
	var targets []provider.Target
	iter(func(t provider.Target, err error) bool {
		if err != nil {
			fmt.Printf("SIMBAD ERR: %v\n", err)
			return false
		}
		targets = append(targets, t)
		// Try to read up to 10
		return len(targets) < 10
	})
	return targets
}

// ResolveObject provides an async streaming mechanism using ADQL.
func (p *Provider) ResolveObject(ctx context.Context, req provider.ObjectRequest) provider.SeqIterator[provider.Target] {
	// 1. Check Cache First
	cacheKey := "resolve:" + provider.Normalize(req.Query) + ":" + string(rune(req.Limit))
	if req.Limit <= 0 {
		cacheKey = "resolve:" + provider.Normalize(req.Query) + ":10"
	}

	if seq, ok := p.cache.Get(cacheKey); ok {
		return seq
	}

	adql := BuildResolveQuery(req)
	body := TAPRequest(adql)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, tapSyncURL, strings.NewReader(body))
	if err != nil {
		return provider.SliceSeq([]provider.Target{})
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return func(yield func(provider.Target, error) bool) {
		resp, err := p.client.Do(httpReq)
		if err != nil {
			yield(provider.Target{}, err)
			return
		}
		if resp.StatusCode >= 400 {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			yield(provider.Target{}, fmt.Errorf("http error %d: %s", resp.StatusCode, string(b)))
			return
		}
		defer resp.Body.Close()

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			yield(provider.Target{}, err)
			return
		}

		targets, err := ParseCSV(strings.NewReader(string(data)))
		if err != nil {
			yield(provider.Target{}, err)
			return
		}

		// 2. Cache Results on successful fetch
		if err := p.cache.Set(cacheKey, targets); err != nil {
			yield(provider.Target{}, err)
			return
		}

		for _, t := range targets {
			if !yield(t, nil) {
				return
			}
		}
	}
}
