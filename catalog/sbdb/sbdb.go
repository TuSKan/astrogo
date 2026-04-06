package sbdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/TuSKan/astrogo/catalog"
)

var sbdbQueryAPI = "https://ssd-api.jpl.nasa.gov/sbdb.api"

// Provider implements catalog.Provider and catalog.ObjectResolver for SBDB.
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

func (p *Provider) Name() string {
	return "sbdb"
}

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
	req := catalog.ObjectRequest{Query: query, Limit: 1}

	iter := p.ResolveObject(ctx, req)
	var targets []catalog.Target
	iter(func(t catalog.Target, err error) bool {
		if err == nil {
			targets = append(targets, t)
		}
		return len(targets) < 1
	})
	return targets
}

func (p *Provider) ResolveObject(ctx context.Context, req catalog.ObjectRequest) catalog.SeqIterator[catalog.Target] {
	queryKey := catalog.Normalize(req.Query)
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
			Object *struct {
				SpkId    string `json:"spkid"`
				FullName string `json:"fullname"`
				Des      string `json:"des"`
				Kind     string `json:"kind"`
			} `json:"object"`
			Message string `json:"message"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			yield(catalog.Target{}, err)
			return
		}

		if payload.Message != "" {
			// This means either multiple matches or error
			// The JSON payload includes generic text if multiple
			// We skip multiple matching to keep it exact resolution for lookup API
			yield(catalog.Target{}, fmt.Errorf("sbdb: %s", payload.Message))
			return
		}

		if payload.Object == nil {
			yield(catalog.Target{}, nil) // empty
			return
		}

		kindStr := "Asteroid"
		if payload.Object.Kind == "c" {
			kindStr = "Comet"
		}

		t := catalog.Target{
			ID:          payload.Object.SpkId,
			Name:        payload.Object.FullName,
			Designation: payload.Object.Des,
			SPKID:       payload.Object.SpkId,
			Kind:        catalog.Kind(kindStr),
			Catalog:     "sbdb",
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
