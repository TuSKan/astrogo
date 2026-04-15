package jpl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

const horizonsAPI = "https://ssd.jpl.nasa.gov/api/horizons.api"

// Provider implements resolve.Provider for major bodies via JPL Horizons.
type Provider struct {
	client *resolve.Client
	cache  resolve.Cache
}

func New() *Provider {
	return &Provider{
		client: resolve.NewClient(),
		cache:  resolve.NewArrowCache(),
	}
}

func (p *Provider) Name() string { return "jpl" }

func (p *Provider) Capabilities() []resolve.Capability {
	return []resolve.Capability{resolve.CapObjectResolution}
}

func (p *Provider) Resolve(query string) (resolve.Target, bool) {
	targets := p.Search(query)
	if len(targets) > 0 {
		return targets[0], true
	}
	return resolve.Target{}, false
}

func (p *Provider) Search(query string) []resolve.Target {
	ctx := context.TODO()
	req := resolve.ObjectRequest{Query: query, Limit: 10}

	iter := p.ResolveObject(ctx, req)
	var targets []resolve.Target
	iter(func(t resolve.Target, err error) bool {
		if err == nil {
			targets = append(targets, t)
		}
		return len(targets) < 10
	})
	return targets
}

func (p *Provider) ResolveObject(ctx context.Context, req resolve.ObjectRequest) resolve.SeqIterator[resolve.Target] {
	queryKey := resolve.Normalize(req.Query)
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
		return resolve.SliceSeq([]resolve.Target{})
	}

	return func(yield func(resolve.Target, error) bool) {
		resp, err := p.client.Do(httpReq)
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}
		defer resp.Body.Close()

		var payload struct {
			Result string `json:"result"`
			Error  string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			yield(resolve.Target{}, err)
			return
		}

		if payload.Error != "" {
			yield(resolve.Target{}, fmt.Errorf("jpl: %s", payload.Error))
			return
		}

		// A real implementation requires parsing the 'Result' text block
		// to extract 'Number', 'Name', and 'Designation' lines reliably.
		// For now we map a heuristic fallback returning the query string itself.
		t := resolve.Target{
			ID:      req.Query,
			Name:    req.Query + " (Horizons metadata parsing stub)",
			Kind:    resolve.KindPlanet,
			Catalog: "jpl_horizons",
		}

		if err := p.cache.Set(cacheKey, []resolve.Target{t}); err != nil {
			yield(resolve.Target{}, err)
			return
		}

		if !yield(t, nil) {
			return
		}
	}
}
