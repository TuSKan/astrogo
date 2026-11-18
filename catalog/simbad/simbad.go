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
// It is not Search's first result: Search matches any object holding the name
// as a substring of any identifier, and for "M31" the first ten of 15,843 are
// Chandra sources inside the galaxy, M31 itself not among them. Resolve
// matches identifiers exactly, against the spellings SIMBAD is known
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
// An error from the iterator is returned, so a CDS outage or a canceled
// context is not read as "not found".
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
// query/parse path directly). Transients are the exception: a supernova's or
// a nova's catalog magnitude is its outburst's, not the brightness of
// anything in the sky now, so they are left out.
//
// With req.Limit set it returns at most that many. Without one it returns
// every object, up to 50,000, SIMBAD's own default output limit; a bound
// faint enough to pass that (about V 8.1) gets the 50,000 brightest followed
// by an error wrapping [resolve.ErrTruncated], never a short list that looks
// complete.
//
// Each object's Aliases are every identifier SIMBAD lists for it, in SIMBAD's
// spelling, and its MemberOf the main identifier of each object SIMBAD's
// hierarchy makes it a member of: a star system, a cluster. Both are fetched
// once the objects are known, with two further requests per 1,000 of them.
// The identifiers are what matches an object to another catalog's record of
// it, and the memberships what tells a star system's record from its
// components'. If either cannot be fetched, the objects still come, followed
// by an error saying how many lack them and why.
//
// Only a complete list, every object with its identifiers and memberships, is
// cached.
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

// addIdentifiers sets each target's Aliases to the identifiers SIMBAD lists for
// the object with the matching oid, and its MemberOf to what SIMBAD's
// hierarchy makes it a member of, asking about identifierBatch objects at a
// time. A target whose oid is 0 has none to ask about and is left as it is.
//
// The memberships are what tell a star system's record from its components'
// (#1182): SIMBAD keeps alf Cen, at V -0.1, as the parent of alf Cen A and
// alf Cen B, and that V is theirs together.
//
// Identifiers and memberships are asked in two passes, so one failing does not
// cost the other. A batch that fails ends its pass: the targets already given
// that pass's lists keep them, and the error, wrapping
// errIdentifiersUnavailable or errMembershipsUnavailable, counts the ones left
// without.
func (p *Provider) addIdentifiers(ctx context.Context, targets []resolve.Target, oids []int64) error {
	at := make(map[int64]int, len(oids))
	ask := make([]int64, 0, len(oids))

	for i, oid := range oids {
		if oid > 0 {
			at[oid] = i
			ask = append(ask, oid)
		}
	}

	idErr := p.addLists(ctx, ask, buildIdentifierQuery, parseIdentifiers, func(oid int64, list []string) {
		targets[at[oid]].Aliases = list
	})
	if idErr != nil {
		idErr = fmt.Errorf("%w %w, so another catalog may list one of them a second time", errIdentifiersUnavailable, idErr)
	}

	memberErr := p.addLists(ctx, ask, buildMembershipQuery, parseMemberships, func(oid int64, list []string) {
		targets[at[oid]].MemberOf = list
	})
	if memberErr != nil {
		memberErr = fmt.Errorf("%w %w, so a star system may be counted with its own components", errMembershipsUnavailable, memberErr)
	}

	return errors.Join(idErr, memberErr)
}

// addLists asks SIMBAD the query build makes about the objects ask names,
// identifierBatch of them at a time, and hands set each object's list from the
// answer parse reads. A batch that fails ends it, with an error counting the
// objects left without.
func (p *Provider) addLists(ctx context.Context, ask []int64, build func([]int64) string,
	parse func(io.Reader) (map[int64][]string, error), set func(oid int64, list []string),
) error {
	asked := make(map[int64]bool, len(ask))
	for _, oid := range ask {
		asked[oid] = true
	}

	for start := 0; start < len(ask); start += identifierBatch {
		batch := ask[start:min(start+identifierBatch, len(ask))]

		lists, err := p.fetchLists(ctx, build(batch), parse)
		if err != nil {
			return fmt.Errorf("for %d of %d objects: %w", len(ask)-start, len(ask), err)
		}

		for oid, list := range lists {
			if asked[oid] {
				set(oid, list)
			}
		}
	}

	return nil
}

// fetchLists asks SIMBAD one query about a batch of objects and parses its
// answer with parse.
func (p *Provider) fetchLists(ctx context.Context, adql string,
	parse func(io.Reader) (map[int64][]string, error),
) (map[int64][]string, error) {
	body, err := p.client.PostForm(ctx, remote.SIMBAD, "", TAPRequest(adql))
	if err != nil {
		return nil, fmt.Errorf("asking about a batch of objects: %w", err)
	}
	defer func() { _ = body.Close() }()

	return parse(body)
}

// errIdentifiersUnavailable marks a bright-object list whose objects came
// without the identifiers that match them to other catalogs.
var errIdentifiersUnavailable = errors.New("simbad: identifiers unavailable")

// errMembershipsUnavailable marks a bright-object list whose objects came
// without what they are members of, the hierarchy that tells a star system's
// record from its components'.
var errMembershipsUnavailable = errors.New("simbad: memberships unavailable")
