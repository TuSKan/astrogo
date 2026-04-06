package simbad

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/TuSKan/astrogo/catalog"
)

// Provider implements the catalog.Provider and catalog.ObjectResolver
// interfaces interacting with SIMBAD's Table Access Protocol endpoint.
type Provider struct {
	client *catalog.Client
	cache  catalog.Cache
}

// New creates a new SIMBAD ObjectResolver.
func New() *Provider {
	return &Provider{
		client: catalog.NewClient(),
		cache:  catalog.NewArrowCache(),
	}
}

// Name returns the provider's display identifier.
func (p *Provider) Name() string {
	return "simbad"
}


// Capabilities lists the supported remote query capacities.
func (p *Provider) Capabilities() []catalog.Capability {
	return []catalog.Capability{catalog.CapObjectResolution}
}

// Resolve matches a single object by returning the most relevant hit.
// Adheres strictly to catalog.Provider. 
func (p *Provider) Resolve(query string) (catalog.Target, bool) {
	targets := p.Search(query)
	if len(targets) > 0 {
		return targets[0], true
	}
	return catalog.Target{}, false
}

// Search matches all objects closely matching a freeform query.
func (p *Provider) Search(query string) []catalog.Target {
	ctx := context.TODO()
	req := catalog.ObjectRequest{Query: query, Limit: 10}
	
	iter := p.ResolveObject(ctx, req)
	var targets []catalog.Target
	iter(func(t catalog.Target, err error) bool {
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
func (p *Provider) ResolveObject(ctx context.Context, req catalog.ObjectRequest) catalog.SeqIterator[catalog.Target] {
	// 1. Check Cache First
	cacheKey := "resolve:" + catalog.Normalize(req.Query) + ":" + string(rune(req.Limit))
	if req.Limit <= 0 {
		cacheKey = "resolve:" + catalog.Normalize(req.Query) + ":10"
	}
	
	if seq, ok := p.cache.Get(cacheKey); ok {
		return seq
	}

	adql := BuildResolveQuery(req)
	body := TAPRequest(adql)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, tapSyncURL, strings.NewReader(body))
	if err != nil {
		return catalog.SliceSeq([]catalog.Target{})
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return func(yield func(catalog.Target, error) bool) {
		resp, err := p.client.Do(httpReq)
		if err != nil {
			yield(catalog.Target{}, err)
			return
		}
		defer resp.Body.Close()

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			yield(catalog.Target{}, err)
			return
		}

		targets, err := ParseCSV(strings.NewReader(string(data)))
		if err != nil {
			yield(catalog.Target{}, err)
			return
		}

		// 2. Cache Results on successful fetch
		p.cache.Set(cacheKey, targets)

		for _, t := range targets {
			if !yield(t, nil) {
				return
			}
		}
	}
}
