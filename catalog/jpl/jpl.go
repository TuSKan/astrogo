package jpl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/TuSKan/astrogo/catalog/provider"
)

const horizonsAPI = "https://ssd.jpl.nasa.gov/api/horizons.api"

// Provider implements provider.Provider for major bodies via JPL Horizons.
type Provider struct {
	client *provider.Client
	cache  provider.Cache
}

func New() *Provider {
	return &Provider{
		client: provider.NewClient(),
		cache:  provider.NewArrowCache(),
	}
}

func (p *Provider) Name() string { return "jpl" }

func (p *Provider) Capabilities() []provider.Capability {
	return []provider.Capability{provider.CapObjectResolution}
}

func (p *Provider) Resolve(query string) (provider.Target, bool) {
	targets := p.Search(query)
	if len(targets) > 0 {
		return targets[0], true
	}
	return provider.Target{}, false
}

func (p *Provider) Search(query string) []provider.Target {
	ctx := context.TODO()
	req := provider.ObjectRequest{Query: query, Limit: 10}

	iter := p.ResolveObject(ctx, req)
	var targets []provider.Target
	iter(func(t provider.Target, err error) bool {
		if err == nil {
			targets = append(targets, t)
		}
		return len(targets) < 10
	})
	return targets
}

func (p *Provider) ResolveObject(ctx context.Context, req provider.ObjectRequest) provider.SeqIterator[provider.Target] {
	queryKey := provider.Normalize(req.Query)
	cacheKey := "resolve:jpl:" + queryKey

	if seq, ok := p.cache.Get(cacheKey); ok {
		return seq
	}

	api, _ := url.Parse(horizonsAPI)
	params := api.Query()
	params.Set("format", "json")
	params.Set("COMMAND", fmt.Sprintf("'%s'", req.Query))
	api.RawQuery = params.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, api.String(), nil)
	if err != nil {
		return provider.SliceSeq([]provider.Target{})
	}

	return func(yield func(provider.Target, error) bool) {
		resp, err := p.client.Do(httpReq)
		if err != nil {
			yield(provider.Target{}, err)
			return
		}
		defer resp.Body.Close()

		var payload struct {
			Result string `json:"result"`
			Error  string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			yield(provider.Target{}, err)
			return
		}

		if payload.Error != "" {
			yield(provider.Target{}, fmt.Errorf("jpl: %s", payload.Error))
			return
		}

		// A real implementation requires parsing the 'Result' text block
		// to extract 'Number', 'Name', and 'Designation' lines reliably.
		// For now we map a heuristic fallback returning the query string itself.
		t := provider.Target{
			ID:      req.Query,
			Name:    req.Query + " (Horizons metadata parsing stub)",
			Kind:    provider.KindPlanet,
			Catalog: "jpl_horizons",
		}

		if err := p.cache.Set(cacheKey, []provider.Target{t}); err != nil {
			yield(provider.Target{}, err)
			return
		}

		if !yield(t, nil) {
			return
		}
	}
}
