package jpl

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/url"
	"slices"
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
	client := remote.Default()

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

// Resolve returns the target that best matches query, the first of
// [Provider.Search]'s results.
func (p *Provider) Resolve(ctx context.Context, query string) (resolve.Target, error) {
	targets, err := p.Search(ctx, query)
	if err != nil {
		return resolve.Target{}, err
	}

	if len(targets) == 0 {
		return resolve.Target{}, fmt.Errorf("%w: %q in jpl", resolve.ErrNotFound, query)
	}

	return targets[0], nil
}

// searchLimit caps how many targets [Provider.Search] returns.
const searchLimit = 10

// Search returns up to searchLimit of Horizons' matches for query, best
// match first.
//
// Horizons lists an ambiguous query's matches in its own order and matches
// a name anywhere inside another: for "Moon" the Earth-Moon barycenter
// comes before the Moon, and for "ISS" Larissa comes first and the
// International Space Station fourth. So Search ranks the whole table by
// [resolve.Score] over each target's name, ID, designation and aliases,
// keeping Horizons' order between equal scores, and only then applies the
// cap: Horizons answers "ACE" with 255 bodies, and lists ACE itself 53rd.
func (p *Provider) Search(ctx context.Context, query string) ([]resolve.Target, error) {
	targets, err := resolve.Drain(p.ResolveObject(ctx, resolve.ObjectRequest{Query: query}), 0)
	if err != nil {
		return nil, fmt.Errorf("searching for %q: %w", query, err)
	}

	targets = rankByMatch(query, targets)

	return targets[:min(len(targets), searchLimit)], nil
}

// rankByMatch returns targets ordered best match for query first, by the
// best [resolve.Score] over each target's name, ID, designation and
// aliases. The sort is stable, so equal scores keep Horizons' order.
func rankByMatch(query string, targets []resolve.Target) []resolve.Target {
	type ranked struct {
		target resolve.Target
		score  float64
	}

	rs := make([]ranked, len(targets))

	for i, t := range targets {
		best := 0.0
		for _, s := range append([]string{t.Name, t.ID, t.Designation}, t.Aliases...) {
			best = max(best, resolve.Score(query, s))
		}

		rs[i] = ranked{t, best}
	}

	slices.SortStableFunc(rs, func(a, b ranked) int { return cmp.Compare(b.score, a.score) })

	out := make([]resolve.Target, len(rs))
	for i, r := range rs {
		out[i] = r.target
	}

	return out
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
