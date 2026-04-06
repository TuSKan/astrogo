package jpl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/TuSKan/astrogo/catalog"
)

const horizonsAPI = "https://ssd.jpl.nasa.gov/api/horizons.api"

// Provider implements catalog.Provider for major bodies via JPL Horizons.
type Provider struct {
	client *catalog.Client
	cache  catalog.Cache
}

func New() *Provider {
	return &Provider{
		client: catalog.NewClient(),
		cache:  catalog.NewArrowCache(),
	}
}

func (p *Provider) Name() string { return "jpl" }

func (p *Provider) Capabilities() []catalog.Capability {
	return []catalog.Capability{catalog.CapObjectResolution}
}

func (p *Provider) Resolve(query string) (catalog.Target, bool) {
	targets := p.Search(query)
	if len(targets) > 0 {
		return targets[0], true
	}
	return catalog.Target{}, false
}

func (p *Provider) Search(query string) []catalog.Target {
	ctx := context.TODO()
	req := catalog.ObjectRequest{Query: query, Limit: 10}

	iter := p.ResolveObject(ctx, req)
	var targets []catalog.Target
	iter(func(t catalog.Target, err error) bool {
		if err == nil {
			targets = append(targets, t)
		}
		return len(targets) < 10
	})
	return targets
}

func (p *Provider) ResolveObject(ctx context.Context, req catalog.ObjectRequest) catalog.SeqIterator[catalog.Target] {
	queryKey := catalog.Normalize(req.Query)
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
		return catalog.SliceSeq([]catalog.Target{})
	}

	return func(yield func(catalog.Target, error) bool) {
		resp, err := p.client.Do(httpReq)
		if err != nil {
			yield(catalog.Target{}, err)
			return
		}
		defer resp.Body.Close()

		var payload struct {
			Result string `json:"result"`
			Error  string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			yield(catalog.Target{}, err)
			return
		}

		if payload.Error != "" {
			yield(catalog.Target{}, fmt.Errorf("jpl: %s", payload.Error))
			return
		}

		// A real implementation requires parsing the 'Result' text block
		// to extract 'Number', 'Name', and 'Designation' lines reliably.
		// For now we map a heuristic fallback returning the query string itself.
		t := catalog.Target{
			ID:      req.Query,
			Name:    req.Query + " (Horizons metadata parsing stub)",
			Kind:    catalog.KindPlanet,
			Catalog: "jpl_horizons",
		}

		if err := p.cache.Set(cacheKey, []catalog.Target{t}); err != nil {
			yield(catalog.Target{}, err)
			return
		}

		if !yield(t, nil) {
			return
		}
	}
}
