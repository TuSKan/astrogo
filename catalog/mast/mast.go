package mast

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
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
func (p *Provider) Resolve(ctx context.Context, query string) (resolve.Target, bool) {
	targets := p.Search(ctx, query)
	if len(targets) > 0 {
		return targets[0], true
	}

	return resolve.Target{}, false
}

// Search searches for targets matching the query.
func (p *Provider) Search(ctx context.Context, query string) []resolve.Target {
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
		"format":  "json",
		"params": map[string]string{
			"input": req.Query,
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

		data, err := io.ReadAll(body)
		if err != nil {
			yield(resolve.Target{}, fmt.Errorf("mast: read response: %w", err))
			return
		}

		// The request above always sets "format": "json", but MAST has been
		// observed to ignore it and return its default XML body anyway —
		// sniff the actual content instead of trusting the requested format.
		var targets []resolve.Target

		if trimmed := bytes.TrimSpace(data); len(trimmed) > 0 && trimmed[0] == '<' {
			var xmlPayload struct {
				ResolvedCoordinate []struct {
					CanonicalName string   `xml:"canonicalName"`
					Resolver      string   `xml:"resolver"`
					RA            *float64 `xml:"ra"`
					Dec           *float64 `xml:"dec"`
				} `xml:"resolvedCoordinate"`
			}

			if err := xml.Unmarshal(data, &xmlPayload); err != nil {
				yield(resolve.Target{}, fmt.Errorf("mast: decode XML response: %w", err))
				return
			}

			targets = make([]resolve.Target, 0, len(xmlPayload.ResolvedCoordinate))
			for _, match := range xmlPayload.ResolvedCoordinate {
				targets = append(targets, newMASTTarget(match.CanonicalName, match.Resolver, match.RA, match.Dec))
			}
		} else {
			var jsonPayload struct {
				Status             string `json:"status"`
				Msg                string `json:"msg"`
				ResolvedCoordinate []struct {
					CanonicalName string   `json:"canonicalName"`
					Resolver      string   `json:"resolver"`
					RA            *float64 `json:"ra"`
					Decl          *float64 `json:"decl"`
				} `json:"resolvedCoordinate"`
			}

			if err := json.Unmarshal(data, &jsonPayload); err != nil {
				yield(resolve.Target{}, fmt.Errorf("mast: decode response: %w", err))
				return
			}

			if jsonPayload.Status == "ERROR" {
				yield(resolve.Target{}, fmt.Errorf("%w: %s", ErrAPIError, jsonPayload.Msg))
				return
			}

			targets = make([]resolve.Target, 0, len(jsonPayload.ResolvedCoordinate))
			for _, match := range jsonPayload.ResolvedCoordinate {
				targets = append(targets, newMASTTarget(match.CanonicalName, match.Resolver, match.RA, match.Decl))
			}
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

// newMASTTarget builds a resolve.Target from one decoded resolvedCoordinate
// match, in either the XML or JSON response shape.
//
// ra/dec are pointers rather than bare float64 so a genuinely-absent field
// (no <ra> element, no "ra" JSON key) decodes to nil instead of a
// masquerading 0.0 — HasCoord is only set true when both are actually
// present, matching how every other astrogo catalog provider treats a
// missing position as absent rather than real.
//
// Catalog is always "mast" (consistent with every other provider setting
// Catalog to its own name), never the relayed sub-resolver name (NED,
// Simbad, VizieR) MAST's Name.Lookup service internally used to answer —
// that information isn't discarded, it's preserved as an alias instead, so
// Catalog keeps one consistent meaning ("which provider produced this row")
// across the whole package.
//
// Epoch defaults to time.J2000 as a best-effort assumption: the API doesn't
// report which sub-resolver's native epoch actually answered, but
// SIMBAD/NED name-lookup responses are conventionally J2000.
func newMASTTarget(canonicalName, resolver string, ra, dec *float64) resolve.Target {
	t := resolve.Target{
		ID:      canonicalName,
		Name:    canonicalName,
		Catalog: "mast",
		Epoch:   time.J2000,
	}

	if resolver != "" {
		t.Aliases = []string{resolver}
	}

	if ra != nil && dec != nil {
		t.Coord = coord.NewICRS(angle.Deg(*ra), angle.Deg(*dec))
		t.HasCoord = true
	}

	return t
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
