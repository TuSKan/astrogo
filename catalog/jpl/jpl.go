package jpl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

// ErrAPIError indicates a JPL Horizons API error response.
var ErrAPIError = errors.New("jpl: API error")

// ErrNotImplemented indicates a successful Horizons response whose free-text
// "result" block this provider does not yet parse. Horizons has no stable
// schema for this field — it ranges from a major-body match table to a full
// small-body orbital-elements printout, and the exact table header wording
// has been observed to differ across API responses — so a best-effort
// heuristic parse here risks silently returning wrong data. Until a
// dedicated parser is verified against live responses for every shape,
// callers get this explicit error instead of a fabricated placeholder Target.
var ErrNotImplemented = errors.New("jpl: Horizons result parsing not implemented for this response")

const horizonsAPI = "https://ssd.jpl.nasa.gov/api/horizons.api"

// Provider implements resolve.Provider for major bodies via JPL Horizons.
type Provider struct {
	client *resolve.Client
	cache  resolve.Cache
}

// New creates a new JPL Horizons catalog provider.
func New() *Provider {
	return &Provider{
		client: resolve.NewClient(),
		cache:  resolve.NewMapCache(),
	}
}

// Name returns the provider identifier.
func (p *Provider) Name() string { return "jpl" }

// Capabilities returns the set of supported resolution operations.
func (p *Provider) Capabilities() []resolve.Capability {
	return []resolve.Capability{resolve.CapObjectResolution}
}

// Resolve performs exact-match resolution for a query.
func (p *Provider) Resolve(query string) (resolve.Target, bool) {
	targets := p.Search(query)
	if len(targets) > 0 {
		return targets[0], true
	}

	return resolve.Target{}, false
}

// Search performs a fuzzy search for the query.
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

// ResolveObject performs streaming resolution via the JPL Horizons API.
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
		return func(yield func(resolve.Target, error) bool) {
			yield(resolve.Target{}, fmt.Errorf("jpl: new request: %w", err))
		}
	}

	return func(yield func(resolve.Target, error) bool) {
		resp, err := p.client.Do(httpReq)
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}
		defer func() {
			cerr := resp.Body.Close()
			if cerr != nil {
				yield(resolve.Target{}, cerr)
			}
		}()

		var payload struct {
			Result string `json:"result"`
			Error  string `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			yield(resolve.Target{}, err)
			return
		}

		if payload.Error != "" {
			yield(resolve.Target{}, fmt.Errorf("%w: %s", ErrAPIError, payload.Error))
			return
		}

		yield(resolve.Target{}, fmt.Errorf("%w: query %q", ErrNotImplemented, req.Query))
	}
}
