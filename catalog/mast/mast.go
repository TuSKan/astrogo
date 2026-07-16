package mast

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/remote"
)

// ErrAPIError indicates a MAST API error response.
var ErrAPIError = errors.New("mast: API error")

// ErrNotImplemented indicates a capability this provider advertises
// (resolve.CapConeSearch) but does not yet implement.
var ErrNotImplemented = errors.New("mast: not implemented")

// Provider implements the resolve.Provider interface for the MAST catalog.
type Provider struct {
	client *remote.Client
	cache  resolve.Cache
}

// New creates a new MAST provider.
func New() *Provider {
	client, err := remote.NewClientFor(remote.MAST)
	if err != nil {
		panic(err) // unregistered endpoint would be a programmer error
	}

	return &Provider{
		client: client,
		cache:  resolve.NewMapCache(),
	}
}

// Name returns the name of the provider.
func (p *Provider) Name() string { return "mast" }

// Capabilities returns the capabilities of the provider.
func (p *Provider) Capabilities() []resolve.Capability {
	return []resolve.Capability{resolve.CapObjectResolution, resolve.CapConeSearch}
}

// Resolve resolves a query to a target.
func (p *Provider) Resolve(query string) (resolve.Target, bool) {
	targets := p.Search(query)
	if len(targets) > 0 {
		return targets[0], true
	}

	return resolve.Target{}, false
}

// Search searches for targets matching the query.
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

// ResolveObject resolves a query to a target.
func (p *Provider) ResolveObject(ctx context.Context, req resolve.ObjectRequest) resolve.SeqIterator[resolve.Target] {
	cacheKey := "resolve:mast:" + resolve.Normalize(req.Query)
	if seq, ok := p.cache.Get(cacheKey); ok {
		return seq
	}

	payload := map[string]any{
		"service": "Mast.Name.Lookup",
		"params": map[string]string{
			"input":  req.Query,
			"format": "json",
		},
	}

	b, err := json.Marshal(payload)
	if err != nil {
		return resolve.SliceSeq([]resolve.Target{})
	}

	v := url.Values{}
	v.Set("request", string(b))

	return func(yield func(resolve.Target, error) bool) {
		body, err := p.client.PostForm(ctx, remote.MAST, "", v)
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}
		defer func() { _ = body.Close() }()

		// The request above always sets "format": "json" (MAST defaults to
		// XML otherwise), so a 2xx response body is always JSON — no
		// format sniffing needed.
		var jsonPayload struct {
			Status             string `json:"status"`
			Msg                string `json:"msg"`
			ResolvedCoordinate []struct {
				CanonicalName string  `json:"canonicalName"`
				Resolver      string  `json:"resolver"`
				RA            float64 `json:"ra"`
				Decl          float64 `json:"decl"`
			} `json:"resolvedCoordinate"`
		}

		if err := json.NewDecoder(body).Decode(&jsonPayload); err != nil {
			yield(resolve.Target{}, fmt.Errorf("mast: decode response: %w", err))
			return
		}

		if jsonPayload.Status == "ERROR" {
			yield(resolve.Target{}, fmt.Errorf("%w: %s", ErrAPIError, jsonPayload.Msg))
			return
		}

		targets := make([]resolve.Target, 0, len(jsonPayload.ResolvedCoordinate))
		for _, match := range jsonPayload.ResolvedCoordinate {
			targets = append(targets, resolve.Target{
				ID:       match.CanonicalName,
				Name:     match.CanonicalName,
				Coord:    coord.NewICRS(angle.Deg(match.RA), angle.Deg(match.Decl)),
				HasCoord: true,
				Catalog:  match.Resolver,
			})
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

// ConeSearch is not yet implemented for CAOM spatial search: STScI's
// Mast.Caom.Cone service has a distinct request/response shape from the
// name-resolution path ResolveObject already handles, unverified against a
// live response so far. Callers get an explicit error rather than a
// fabricated empty-but-successful result.
func (p *Provider) ConeSearch(_ context.Context, _ resolve.ConeRequest) resolve.SeqIterator[resolve.Target] {
	return func(yield func(resolve.Target, error) bool) {
		yield(resolve.Target{}, ErrNotImplemented)
	}
}
