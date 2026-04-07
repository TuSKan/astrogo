package sbdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/TuSKan/astrogo/catalog/provider"
)

var sbdbQueryAPI = "https://ssd-api.jpl.nasa.gov/sbdb.api"

// Provider implements provider.Provider and provider.ObjectResolver for SBDB.
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

func (p *Provider) Name() string {
	return "sbdb"
}

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
	req := provider.ObjectRequest{Query: query, Limit: 1}

	iter := p.ResolveObject(ctx, req)
	var targets []provider.Target
	iter(func(t provider.Target, err error) bool {
		if err == nil {
			targets = append(targets, t)
		}
		return len(targets) < 1
	})
	return targets
}

func (p *Provider) ResolveObject(ctx context.Context, req provider.ObjectRequest) provider.SeqIterator[provider.Target] {
	queryKey := provider.Normalize(req.Query)
	cacheKey := "resolve:sbdb:" + queryKey

	if seq, ok := p.cache.Get(cacheKey); ok {
		return seq
	}

	api, _ := url.Parse(sbdbQueryAPI)
	params := api.Query()

	// Switch to using Lookup API explicitly targeted via sstr
	params.Set("sstr", req.Query)

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
			Object *struct {
				SpkId    string `json:"spkid"`
				FullName string `json:"fullname"`
				Des      string `json:"des"`
				Kind     string `json:"kind"`
			} `json:"object"`
			Message string `json:"message"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			yield(provider.Target{}, err)
			return
		}

		if payload.Message != "" {
			// This means either multiple matches or error
			// The JSON payload includes generic text if multiple
			// We skip multiple matching to keep it exact resolution for lookup API
			yield(provider.Target{}, fmt.Errorf("sbdb: %s", payload.Message))
			return
		}

		if payload.Object == nil {
			yield(provider.Target{}, nil) // empty
			return
		}

		kindStr := "Asteroid"
		if payload.Object.Kind == "c" {
			kindStr = "Comet"
		}

		t := provider.Target{
			ID:          payload.Object.SpkId,
			Name:        payload.Object.FullName,
			Designation: payload.Object.Des,
			SPKID:       payload.Object.SpkId,
			Kind:        provider.Kind(kindStr),
			Catalog:     "sbdb",
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
