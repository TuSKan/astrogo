package sbdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

var sbdbQueryAPI = "https://ssd-api.jpl.nasa.gov/sbdb.api"

// Provider implements resolve.Provider and resolve.ObjectResolver for SBDB.
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

func (p *Provider) Name() string {
	return "sbdb"
}

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
	req := resolve.ObjectRequest{Query: query, Limit: 1}

	iter := p.ResolveObject(ctx, req)
	var targets []resolve.Target
	iter(func(t resolve.Target, err error) bool {
		if err == nil {
			targets = append(targets, t)
		}
		return len(targets) < 1
	})
	return targets
}

func (p *Provider) ResolveObject(ctx context.Context, req resolve.ObjectRequest) resolve.SeqIterator[resolve.Target] {
	queryKey := resolve.Normalize(req.Query)
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
			Object *struct {
				SpkId    string `json:"spkid"`
				FullName string `json:"fullname"`
				Des      string `json:"des"`
				Kind     string `json:"kind"`
			} `json:"object"`
			Message string `json:"message"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			yield(resolve.Target{}, err)
			return
		}

		if payload.Message != "" {
			// This means either multiple matches or error
			// The JSON payload includes generic text if multiple
			// We skip multiple matching to keep it exact resolution for lookup API
			yield(resolve.Target{}, fmt.Errorf("sbdb: %s", payload.Message))
			return
		}

		if payload.Object == nil {
			yield(resolve.Target{}, nil) // empty
			return
		}

		kindStr := "Asteroid"
		if payload.Object.Kind == "c" {
			kindStr = "Comet"
		}

		t := resolve.Target{
			ID:          payload.Object.SpkId,
			Name:        payload.Object.FullName,
			Designation: payload.Object.Des,
			SPKID:       payload.Object.SpkId,
			Kind:        resolve.Kind(kindStr),
			Catalog:     "sbdb",
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
