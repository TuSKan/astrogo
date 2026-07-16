package jpl

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/remote"
)

// ErrAPIError indicates a JPL Horizons API error response.
var ErrAPIError = errors.New("jpl: API error")

// ErrNotImplemented indicates a successful Horizons response whose free-text
// "result" block matches none of the recognized shapes: the major-body
// ambiguous-match table, the small-body DASTCOM index table, or the
// single-match "Target body name:" header line. This provider deliberately
// does not attempt to parse a full orbital-elements printout body — that
// portion of Horizons' output has no stable, verified schema — so a
// genuinely novel response shape surfaces this explicit error instead of a
// guessed/fabricated Target.
var ErrNotImplemented = errors.New("jpl: Horizons result parsing not implemented for this response")

// Provider implements resolve.Provider for major bodies via JPL Horizons.
type Provider struct {
	client *remote.Client
	cache  resolve.Cache
}

// New creates a new JPL Horizons catalog provider.
func New() *Provider {
	client, err := remote.NewClientFor(remote.JPLHorizons)
	if err != nil {
		panic(err) // unregistered endpoint would be a programmer error
	}

	return &Provider{
		client: client,
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

	params := url.Values{}
	params.Set("format", "json")
	params.Set("COMMAND", fmt.Sprintf("'%s'", req.Query))

	return func(yield func(resolve.Target, error) bool) {
		var payload struct {
			Result string `json:"result"`
			Error  string `json:"error"`
		}

		if err := p.client.GetJSON(ctx, remote.JPLHorizons, "", params, &payload); err != nil {
			yield(resolve.Target{}, err)
			return
		}

		if payload.Error != "" {
			yield(resolve.Target{}, fmt.Errorf("%w: %s", ErrAPIError, payload.Error))
			return
		}

		var (
			targets []resolve.Target
			matched bool
		)

		if ts, ok := parseSmallBodyIndexTable(payload.Result); ok {
			targets, matched = ts, true
		} else if ts, ok := parseMajorBodyMatchTable(payload.Result); ok {
			targets, matched = ts, true
		} else if t, ok := parseExactMatch(payload.Result); ok {
			targets, matched = []resolve.Target{t}, true
		}

		if !matched && strings.TrimSpace(payload.Result) != "" {
			yield(resolve.Target{}, fmt.Errorf("%w: query %q", ErrNotImplemented, req.Query))
			return
		}

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
