package simbad

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/remote"
)

// Provider implements the resolve.Provider and resolve.ObjectResolver
// interfaces interacting with SIMBAD's Table Access Protocol endpoint.
type Provider struct {
	client *remote.Client
	cache  resolve.Cache
}

// New creates a new SIMBAD ObjectResolver.
func New() *Provider {
	client := remote.Default()

	return &Provider{
		client: client,
		cache:  resolve.NewMapCache(),
	}
}

// Name returns the provider's display identifier.
func (p *Provider) Name() string {
	return "simbad"
}

// Capabilities lists the supported remote query capacities.
func (p *Provider) Capabilities() []resolve.Capability {
	return []resolve.Capability{resolve.CapObjectResolution, resolve.CapMagnitudeBrowse}
}

// Resolve identifies the single object a name refers to.
//
// # Identity, not the best of a search
//
// This used to be Search's first result after client-side scoring, and the
// scoring could not save it: the underlying query matched any object holding
// the name as a substring of any identifier, so for "M31" the ten rows
// fetched out of 15,843 were Chandra sources inside the galaxy and M31 itself
// was not among them. Scoring ranks what it is given.
//
// It now matches identifiers exactly, against the spellings SIMBAD is known
// to store — see [identifierVariants]. An unknown name returns
// [resolve.ErrNotFound] rather than the least-wrong of ten wrong answers,
// because a caller can act on "not found" and cannot act on a plausible
// object 70 degrees from the one they asked for.
//
// Aliases fan out across rows, so several rows may arrive for one object;
// they are merged by oid upstream in ResolveObject and the first target is
// the object.
func (p *Provider) Resolve(ctx context.Context, query string) (resolve.Target, error) {
	targets, err := p.ResolveObjects(ctx, resolve.ObjectRequest{Query: query, Limit: 10})
	if err != nil {
		return resolve.Target{}, err
	}

	if len(targets) == 0 {
		return resolve.Target{}, fmt.Errorf("%w: %q in SIMBAD", resolve.ErrNotFound, query)
	}

	return targets[0], nil
}

// ResolveObjects drains ResolveObject into a slice — the shape both Resolve
// and Search need.
//
// It used to swallow errors here: the iterator's error was written to the
// global log package and the drained slice came back empty, which every caller
// then read as "not found". A CDS outage, a canceled context and a genuinely
// absent object were indistinguishable, and the only trace was a line on
// whatever writer log.SetOutput last pointed at.
//
// The streaming ResolveObject always carried the error correctly. This is the
// convenience wrapper that used to throw it away.
func (p *Provider) ResolveObjects(ctx context.Context, req resolve.ObjectRequest) ([]resolve.Target, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	targets, err := resolve.Drain(p.ResolveObject(ctx, req), limit)
	if err != nil {
		return nil, fmt.Errorf("resolving %q: %w", req.Query, err)
	}

	return targets, nil
}

// Search matches objects whose identifiers begin with a freeform query.
//
// Distinct from [Provider.Resolve], and deliberately: several answers are
// this method's purpose, where for Resolve they are a failure. Anchored at
// the start rather than wrapped in wildcards, and ordered brightest-first,
// so the rows are ones a person would recognize and are the same rows on
// every call — the query this replaces had no ORDER BY, and two identical
// searches for "M42" returned different objects.
func (p *Provider) Search(ctx context.Context, query string) ([]resolve.Target, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}

	req := resolve.ObjectRequest{Query: query, Limit: 10}
	adql := BuildSearchQuery(req)

	targets, err := resolve.Drain(p.stream(ctx, "search:"+query, adql), req.Limit)
	if err != nil {
		return nil, fmt.Errorf("searching for %q: %w", query, err)
	}

	// No error and no rows is a real answer: the query was asked and matched
	// nothing. Unlike Resolve, that is not ErrNotFound — Search's contract is
	// "the matches I found", and none is a legitimate count.
	return targets, nil
}

// SearchBright returns every SIMBAD object brighter than req.MaxVMag,
// brightest-first — the bulk counterpart to Resolve/Search's name-driven
// lookup (see BuildBrightQuery for why this can't reuse ResolveObject's
// query/parse path directly).
//
// With req.Limit set it returns at most that many. Without one it returns
// every object, up to 50,000, SIMBAD's own default output limit; a bound
// faint enough to pass that (about V 8.1) gets the 50,000 brightest followed
// by an error wrapping [resolve.ErrTruncated], never a short list that looks
// complete.
//
// Each object's Aliases are every identifier SIMBAD lists for it, in SIMBAD's
// spelling, fetched once the objects are known with one further request per
// 1,000 of them: they are what matches an object to another catalog's record
// of it. If they cannot be fetched, the objects still come, those without
// identifiers followed by an error saying how many lack them and why.
//
// Only a complete list, every object with its identifiers, is cached.
func (p *Provider) SearchBright(ctx context.Context, req resolve.BrightRequest) resolve.SeqIterator[resolve.Target] {
	cacheKey := fmt.Sprintf("bright:%f:%d", req.MaxVMag, req.Limit)
	if seq, ok := p.cache.Get(cacheKey); ok {
		return seq
	}

	adql := BuildBrightQuery(req)
	v := TAPRequest(adql)

	// The service stops at its default 50,000 rows unless asked for more, and
	// a CSV answer does not say it stopped. One row past the cap is what
	// tells a cut list from a complete one.
	if req.Limit <= 0 {
		v.Set("MAXREC", strconv.Itoa(brightRowCap+1))
	}

	return func(yield func(resolve.Target, error) bool) {
		body, err := p.client.PostForm(ctx, remote.SIMBAD, "", v)
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}
		defer func() { _ = body.Close() }()

		data, err := io.ReadAll(body)
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}

		targets, oids, err := parseBright(strings.NewReader(string(data)))
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}

		truncated := req.Limit <= 0 && len(targets) > brightRowCap
		if truncated {
			targets, oids = targets[:brightRowCap], oids[:brightRowCap]
		}

		idErr := p.addIdentifiers(ctx, targets, oids)
		if !truncated && idErr == nil {
			if err := p.cache.Set(cacheKey, targets); err != nil {
				yield(resolve.Target{}, err)
				return
			}
		}

		for _, t := range targets {
			if !yield(t, nil) {
				return
			}
		}

		if idErr != nil && !yield(resolve.Target{}, idErr) {
			return
		}

		if truncated {
			yield(resolve.Target{}, fmt.Errorf("%w: SIMBAD has more than %d objects brighter than V %g",
				resolve.ErrTruncated, brightRowCap, req.MaxVMag))
		}
	}
}

// addIdentifiers sets each target's Aliases to the identifiers SIMBAD lists for
// the object with the matching oid, asking about identifierBatch objects at a
// time. A target whose oid is 0 has none to ask about and is left as it is.
//
// A batch that fails ends the fetch: the targets already given identifiers
// keep them, and the error, wrapping errIdentifiersUnavailable, counts the
// ones that have none.
func (p *Provider) addIdentifiers(ctx context.Context, targets []resolve.Target, oids []int64) error {
	at := make(map[int64]int, len(oids))
	ask := make([]int64, 0, len(oids))

	for i, oid := range oids {
		if oid > 0 {
			at[oid] = i
			ask = append(ask, oid)
		}
	}

	for start := 0; start < len(ask); start += identifierBatch {
		batch := ask[start:min(start+identifierBatch, len(ask))]

		ids, err := p.fetchIdentifiers(ctx, batch)
		if err != nil {
			return fmt.Errorf("%w for %d of %d objects, so another catalog may list one of them a second time: %w",
				errIdentifiersUnavailable, len(ask)-start, len(ask), err)
		}

		for oid, list := range ids {
			if i, ok := at[oid]; ok {
				targets[i].Aliases = list
			}
		}
	}

	return nil
}

// fetchIdentifiers asks SIMBAD for the identifiers of one batch of objects.
func (p *Provider) fetchIdentifiers(ctx context.Context, oids []int64) (map[int64][]string, error) {
	body, err := p.client.PostForm(ctx, remote.SIMBAD, "", TAPRequest(buildIdentifierQuery(oids)))
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()

	return parseIdentifiers(body)
}

// errIdentifiersUnavailable marks a bright-object list whose objects came
// without the identifiers that match them to other catalogs.
var errIdentifiersUnavailable = errors.New("simbad: identifiers unavailable")

// ResolveObject provides an async streaming mechanism using ADQL.
func (p *Provider) ResolveObject(ctx context.Context, req resolve.ObjectRequest) resolve.SeqIterator[resolve.Target] {
	// 1. Check Cache First (Maintain case to prevent ADQL case-sensitive collisions)
	cacheKey := fmt.Sprintf("resolve:%s:%d", req.Query, req.Limit)
	if req.Limit <= 0 {
		cacheKey = fmt.Sprintf("resolve:%s:10", req.Query)
	}

	if seq, ok := p.cache.Get(cacheKey); ok {
		return seq
	}

	adql := BuildResolveQuery(req)
	if adql == "" {
		return func(yield func(resolve.Target, error) bool) {
			yield(resolve.Target{}, ErrEmptyQuery)
		}
	}

	return p.stream(ctx, cacheKey, adql)
}

// stream runs one ADQL query and yields its rows, caching a successful
// fetch. Shared by the identity and search paths, which differ only in the
// query they hand it.
func (p *Provider) stream(ctx context.Context, cacheKey, adql string) resolve.SeqIterator[resolve.Target] {
	v := TAPRequest(adql)

	return func(yield func(resolve.Target, error) bool) {
		body, err := p.client.PostForm(ctx, remote.SIMBAD, "", v)
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}
		defer func() { _ = body.Close() }()

		data, err := io.ReadAll(body)
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}

		targets, err := ParseCSV(strings.NewReader(string(data)))
		if err != nil {
			yield(resolve.Target{}, err)
			return
		}

		// 2. Cache Results on successful fetch
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
