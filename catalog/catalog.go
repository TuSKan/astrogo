package catalog

import (
	"context"
	"errors"
	"sort"
	"sync"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/gaia"
	"github.com/TuSKan/astrogo/catalog/jpl"
	"github.com/TuSKan/astrogo/catalog/mast"
	"github.com/TuSKan/astrogo/catalog/norad"
	"github.com/TuSKan/astrogo/catalog/openngc"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/catalog/sbdb"
	"github.com/TuSKan/astrogo/catalog/simbad"
	"github.com/TuSKan/astrogo/catalog/vizier"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// Source represents an astronomical data provider type.
type Source int

const (
	// OpenNGC is the embedded OpenNGC deep-sky catalog.
	OpenNGC Source = iota
	// SIMBAD is the CDS SIMBAD astronomical database.
	SIMBAD
	// MAST is the Mikulski Archive for Space Telescopes.
	MAST
	// JPL is the NASA JPL Horizons ephemeris service.
	JPL
	// SBDB is the NASA JPL Small-Body Database.
	SBDB
	// Gaia is the ESA Gaia DR3 catalog via TAP.
	Gaia
	// VizieR is the CDS VizieR catalog service.
	VizieR
	// NORAD is the NORAD space-track satellite catalog.
	NORAD
)

var (
	// ErrNotFound is returned when no catalog provider can resolve a query.
	ErrNotFound = errors.New("target not found")
	// ErrAmbiguous is returned when a query matches multiple targets.
	ErrAmbiguous = errors.New("ambiguous target name")
)

// Target and related types are re-exported from the resolve package.
type (
	// Target is a resolved astronomical target.
	Target = resolve.Target
	// Provider is a catalog data source.
	Provider = resolve.Provider
	// Kind is the classification of an astronomical object.
	Kind = resolve.Kind
	// ObjectRequest is a query for resolving objects.
	ObjectRequest = resolve.ObjectRequest
	// SeqIterator is a streaming result iterator.
	SeqIterator[T any] = resolve.SeqIterator[T]
)

// defaultPositionMatchThreshold is the maximum angular separation (after
// epoch normalization) at which two Targets from different providers,
// sharing no alias or ID, are still considered the same physical object.
// 2 arcsec is generous against typical SIMBAD/OpenNGC/MAST-relayed position
// precision, and tight against typical inter-object separation in dense
// fields — see Resolver.PositionMatchThreshold to override it.
var defaultPositionMatchThreshold = angle.Arcsec(2)

// defaultCap is Search's default result-count ceiling — see Resolver.Limit.
const defaultCap = 10

// coneSearchRadiusFactor widens a ConeSearch bridge query beyond the
// acceptance threshold (bounding candidate retrieval, not match
// acceptance — retrieved candidates are still filtered against the exact
// threshold before being accepted into a group).
const coneSearchRadiusFactor = 2.5

type resolverConfig struct {
	positionMatchThreshold angle.Angle
	cap                    int
}

// coneProvider pairs a resolve.ConeSearcher with the provider name
// (Provider.Name()) that produced it — ConeSearcher itself has no Name()
// method, so this is captured once at construction from the same
// concrete provider value that also satisfies Provider.
type coneProvider struct {
	name string
	cs   resolve.ConeSearcher
}

// Resolver orchestrates multiple providers to find astronomical targets.
//
// Resolve and Search cross-match every provider's results for the same
// query — by shared alias/ID first, falling back to angular separation
// (after epoch-normalizing via proper motion) when alias sets don't
// overlap — and merge each group of matching Targets field-by-field from a
// provider-precedence table, rather than returning one provider's whole,
// possibly-incomplete record. Registered ConeSearcher providers (Gaia,
// VizieR) are bridged in around each group's anchor position, so their
// astrometry can participate even though neither does name-based lookup.
type Resolver struct {
	providers     []Provider
	coneSearchers []coneProvider
	cfg           resolverConfig
}

// NewResolver instantiates remote and local catalog implementations
// securely for the given sources. Use Limit/PositionMatchThreshold on the
// returned Resolver to override their defaults (10 results, 2 arcsec).
func NewResolver(sources ...Source) *Resolver {
	cfg := resolverConfig{
		positionMatchThreshold: defaultPositionMatchThreshold,
		cap:                    defaultCap,
	}

	var providers []Provider

	for _, src := range sources {
		switch src {
		case OpenNGC:
			providers = append(providers, openngc.New())
		case SIMBAD:
			providers = append(providers, simbad.New())
		case MAST:
			providers = append(providers, mast.New())
		case JPL:
			providers = append(providers, jpl.New())
		case SBDB:
			providers = append(providers, sbdb.New())
		case Gaia:
			providers = append(providers, gaia.New())
		case VizieR:
			providers = append(providers, vizier.New())
		case NORAD:
			providers = append(providers, norad.New())
		}
	}

	var coneSearchers []coneProvider

	for _, p := range providers {
		if cs, ok := p.(resolve.ConeSearcher); ok {
			coneSearchers = append(coneSearchers, coneProvider{name: p.Name(), cs: cs})
		}
	}

	return &Resolver{providers: providers, coneSearchers: coneSearchers, cfg: cfg}
}

// Limit sets the maximum number of results Search returns (default 10) and
// returns r for chaining.
func (r *Resolver) Limit(n int) *Resolver {
	r.cfg.cap = n
	return r
}

// PositionMatchThreshold sets the maximum angular separation (after epoch
// normalization) at which two Targets are considered the same object
// (default 2 arcsec) and returns r for chaining.
func (r *Resolver) PositionMatchThreshold(threshold angle.Angle) *Resolver {
	r.cfg.positionMatchThreshold = threshold
	return r
}

// candidate pairs a Target with the name of the provider (Provider.Name(),
// never Target.Catalog) that produced it.
type candidate struct {
	target   Target
	provider string
}

// Resolve finds a single target matching the query, merged across every
// provider that resolves it directly.
func (r *Resolver) Resolve(ctx context.Context, query string) (Target, error) {
	q := resolve.Normalize(query)
	if q == "" {
		return Target{}, ErrNotFound
	}

	var candidates []candidate

	for _, p := range r.providers {
		if t, ok := p.Resolve(ctx, query); ok {
			candidates = append(candidates, candidate{t, p.Name()})
		}
	}

	if len(candidates) == 0 {
		results := r.Search(ctx, query)
		if len(results) > 0 {
			return results[0], nil
		}

		return Target{}, ErrNotFound
	}

	groups := r.reconcile(ctx, candidates)

	// candidates (and therefore groups) preserve provider-registration
	// order, so the first group corresponds to the highest-priority
	// provider's hit unless it was merged with a later one.
	return groups[0], nil
}

// Search returns all matching, cross-matched-and-merged targets from every
// provider, ranked by name/alias/ID match quality and capped at Limit's
// configured limit (default 10).
func (r *Resolver) Search(ctx context.Context, query string) []Target {
	q := resolve.Normalize(query)
	if q == "" {
		return nil
	}

	var candidates []candidate

	for _, p := range r.providers {
		for _, t := range p.Search(ctx, query) {
			candidates = append(candidates, candidate{t, p.Name()})
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	groups := r.reconcile(ctx, candidates)

	type scoredTarget struct {
		t     Target
		score float64
	}

	scored := make([]scoredTarget, len(groups))

	for i, t := range groups {
		bestScore := resolve.Score(q, t.Name)
		for _, alias := range t.Aliases {
			if s := resolve.Score(q, alias); s > bestScore {
				bestScore = s
			}
		}

		if s := resolve.Score(q, t.ID); s > bestScore {
			bestScore = s
		}

		scored[i] = scoredTarget{t, bestScore}
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	limit := min(len(scored), r.cfg.cap)

	final := make([]Target, limit)
	for i := range limit {
		final[i] = scored[i].t
	}

	return final
}

// ── Cross-match + merge pipeline ─────────────────────────────────────────

// reconcile cross-matches candidates describing the same physical object
// (by shared alias/ID, falling back to position after epoch-normalizing),
// bridges any registered ConeSearcher's astrometry in around each group's
// trustworthy anchor position, and merges each resulting group into one
// Target via field-level provider precedence.
func (r *Resolver) reconcile(ctx context.Context, candidates []candidate) []Target {
	uf := newUnionFind(len(candidates))
	unionByAlias(candidates, uf)
	unionByPosition(candidates, uf, r.cfg.positionMatchThreshold)

	groups := groupCandidates(candidates, uf)

	if len(r.coneSearchers) > 0 {
		groups = r.bridgeConeSearch(ctx, groups)
	}

	merged := make([]Target, len(groups))
	for i, g := range groups {
		merged[i] = mergeGroup(g)
	}

	return merged
}

// group holds every candidate believed to describe the same physical
// object, in provider-registration order.
type group struct {
	candidates []candidate
}

// ── Union-find ────────────────────────────────────────────────────────────

type unionFind struct {
	parent []int
	rank   []int
}

func newUnionFind(n int) *unionFind {
	uf := &unionFind{parent: make([]int, n), rank: make([]int, n)}
	for i := range uf.parent {
		uf.parent[i] = i
	}

	return uf
}

func (uf *unionFind) find(x int) int {
	for uf.parent[x] != x {
		uf.parent[x] = uf.parent[uf.parent[x]] // path halving
		x = uf.parent[x]
	}

	return x
}

func (uf *unionFind) union(a, b int) {
	ra, rb := uf.find(a), uf.find(b)
	if ra == rb {
		return
	}

	if uf.rank[ra] < uf.rank[rb] {
		ra, rb = rb, ra
	}

	uf.parent[rb] = ra
	if uf.rank[ra] == uf.rank[rb] {
		uf.rank[ra]++
	}
}

// ── Alias-graph cross-match (primary signal) ─────────────────────────────

type matchKey struct {
	kind  byte // 'i' for ID, 'a' for alias — both funnel into the same union pass
	value string
}

// unionByAlias generalizes bigsky's "join on shared Tycho/Hipparcos ID" to
// Go's Aliases []string: every candidate's own ID and every alias it
// carries are indexed together, and any two candidates sharing a bucket
// are unioned. SIMBAD's own cross-identifier table is the one place
// another provider's native ID format already shows up inside a
// different provider's Aliases, which is what makes this work in
// practice for e.g. a SIMBAD hit and a Gaia ConeSearch hit of the same
// star.
func unionByAlias(candidates []candidate, uf *unionFind) {
	idx := make(map[matchKey][]int)

	add := func(k matchKey, i int) { idx[k] = append(idx[k], i) }

	for i, c := range candidates {
		if c.target.ID != "" {
			add(matchKey{'i', resolve.Normalize(c.target.ID)}, i)
		}

		for _, a := range c.target.Aliases {
			if a != "" {
				add(matchKey{'a', resolve.Normalize(a)}, i)
			}
		}
	}

	for _, indices := range idx {
		for i := 1; i < len(indices); i++ {
			uf.union(indices[0], indices[i])
		}
	}
}

// ── Positional fallback cross-match (secondary signal) ───────────────────

// trustworthyCoord reports whether c's Coord should be trusted for
// positional cross-matching. Gaia and MAST have both been observed to set
// HasCoord unconditionally even when the underlying parse/decode failed;
// this is defense in depth on top of the upstream fixes to those
// providers, cheap and correct regardless of whether a future regression
// reintroduces that class of bug.
func trustworthyCoord(t Target, provider string) bool {
	if !t.HasCoord {
		return false
	}

	switch provider {
	case "gaia", "mast":
		return !t.Coord.IsZero()
	default:
		return true
	}
}

// singletonIndices returns the indices whose union-find group currently
// has size 1 — the only candidates eligible for positional matching,
// since anything the alias pass already grouped needs no further check.
func singletonIndices(candidates []candidate, uf *unionFind) []int {
	counts := make(map[int]int, len(candidates))
	for i := range candidates {
		counts[uf.find(i)]++
	}

	out := make([]int, 0, len(candidates))

	for i := range candidates {
		if counts[uf.find(i)] == 1 {
			out = append(out, i)
		}
	}

	return out
}

// unionByPosition epoch-normalizes every still-singleton, trustworthy-coord
// candidate to J2000 and unions any pair within threshold. O(M²) over the
// remaining singletons M, which is bounded by provider count (not catalog
// size) at this point in the pipeline — appropriate at this scale; see
// catalog/doc.go for the fuller justification against a spatial index.
func unionByPosition(candidates []candidate, uf *unionFind, threshold angle.Angle) {
	singletons := singletonIndices(candidates, uf)

	type normalized struct {
		idx int
		c   coord.ICRS
	}

	norm := make([]normalized, 0, len(singletons))

	for _, i := range singletons {
		cand := candidates[i]
		if !trustworthyCoord(cand.target, cand.provider) {
			continue
		}

		propagated, err := coord.PropagateEpoch(cand.target.Coord, cand.target.Epoch, time.J2000)
		if err != nil {
			continue
		}

		norm = append(norm, normalized{i, propagated})
	}

	for a := range norm {
		for b := a + 1; b < len(norm); b++ {
			if uf.find(norm[a].idx) == uf.find(norm[b].idx) {
				continue
			}

			if coord.Separation(norm[a].c, norm[b].c) <= threshold {
				uf.union(norm[a].idx, norm[b].idx)
			}
		}
	}
}

// groupCandidates assembles union-find groups, preserving first-seen-root
// order so group 0 corresponds to the highest-priority (first-registered)
// provider's candidate unless it was merged with a later one.
func groupCandidates(candidates []candidate, uf *unionFind) []group {
	byRoot := make(map[int]*group, len(candidates))

	var order []int

	for i, c := range candidates {
		root := uf.find(i)

		g, ok := byRoot[root]
		if !ok {
			g = &group{}
			byRoot[root] = g

			order = append(order, root)
		}

		g.candidates = append(g.candidates, c)
	}

	groups := make([]group, len(order))
	for i, root := range order {
		groups[i] = *byRoot[root]
	}

	return groups
}

// ── ConeSearch bridge (Gaia/VizieR) ──────────────────────────────────────

// anchorCoord returns the first trustworthy, epoch-normalized position in
// g, if any — the center a ConeSearch bridge query is built around.
func anchorCoord(g group) (coord.ICRS, bool) {
	for _, c := range g.candidates {
		if !trustworthyCoord(c.target, c.provider) {
			continue
		}

		propagated, err := coord.PropagateEpoch(c.target.Coord, c.target.Epoch, time.J2000)
		if err != nil {
			continue
		}

		return propagated, true
	}

	return coord.ICRS{}, false
}

// bridgeConeSearch fires a ConeSearch against every registered
// ConeSearcher around each group's anchor position (concurrently, ctx
// cancellable), then folds any result within threshold of that anchor
// into the group. Groups with no anchor position (e.g. JPL/SBDB-only
// matches, which never set Coord) never trigger a ConeSearch call.
func (r *Resolver) bridgeConeSearch(ctx context.Context, groups []group) []group {
	type job struct {
		groupIdx int
		anchor   coord.ICRS
	}

	var jobs []job

	for i, g := range groups {
		if anchor, ok := anchorCoord(g); ok {
			jobs = append(jobs, job{i, anchor})
		}
	}

	if len(jobs) == 0 {
		return groups
	}

	extra := make([][]candidate, len(groups))
	radius := r.cfg.positionMatchThreshold.MulScalar(coneSearchRadiusFactor)

	var wg sync.WaitGroup

	var mu sync.Mutex

	for _, j := range jobs {
		for _, cp := range r.coneSearchers {
			wg.Add(1)

			go func(groupIdx int, anchor coord.ICRS, cp coneProvider) {
				defer wg.Done()

				req := resolve.ConeRequest{Center: anchor, Radius: radius, Limit: 20}

				var found []candidate

				iter := cp.cs.ConeSearch(ctx, req)
				iter(func(t resolve.Target, err error) bool {
					if err == nil {
						found = append(found, candidate{t, cp.name})
					}

					return true
				})

				if len(found) == 0 {
					return
				}

				mu.Lock()

				extra[groupIdx] = append(extra[groupIdx], found...)

				mu.Unlock()
			}(j.groupIdx, j.anchor, cp)
		}
	}

	wg.Wait()

	for i := range groups {
		if len(extra[i]) == 0 {
			continue
		}

		groups[i] = foldConeSearchResults(groups[i], extra[i], r.cfg.positionMatchThreshold)
	}

	return groups
}

// foldConeSearchResults filters newCandidates (a bridge query's raw
// results, retrieved within the wider coneSearchRadiusFactor radius) down
// to those genuinely within threshold of g's own anchor, and appends them
// to g.
func foldConeSearchResults(g group, newCandidates []candidate, threshold angle.Angle) group {
	anchor, ok := anchorCoord(g)
	if !ok {
		return g
	}

	for _, nc := range newCandidates {
		if !trustworthyCoord(nc.target, nc.provider) {
			continue
		}

		propagated, err := coord.PropagateEpoch(nc.target.Coord, nc.target.Epoch, time.J2000)
		if err != nil {
			continue
		}

		if coord.Separation(anchor, propagated) <= threshold {
			g.candidates = append(g.candidates, nc)
		}
	}

	return g
}

// ── Field-level precedence merge ─────────────────────────────────────────

// fieldRule coalesces one field (or a tightly-coupled cluster of fields)
// from whichever provider in a group ranks highest in precedence and
// actually has it populated.
type fieldRule struct {
	precedence []string
	hasField   func(Target) bool
	take       func(dst *Target, src Target, provider string)
}

func setProvenance(dst *Target, field, provider string) {
	if dst.Provenance == nil {
		dst.Provenance = make(map[string]string)
	}

	dst.Provenance[field] = provider
}

// scalarFieldRules covers fields whose precedence is independent of
// Coord/Parallax/PmRA/PmDec (handled separately in mergeGroup, since
// those four are astrometrically coupled and must come from the same
// source or not at all).
var scalarFieldRules = []fieldRule{
	{
		// Gaia's VMag is a derived G->V estimate, not real photometry —
		// ranks last despite leading astrometry precedence below.
		precedence: []string{"simbad", "openngc", "gaia"},
		hasField:   func(t Target) bool { return t.HasVMag },
		take: func(dst *Target, src Target, provider string) {
			dst.VMag, dst.HasVMag = src.VMag, true
			setProvenance(dst, "VMag", provider)
		},
	},
	{
		precedence: []string{"gaia", "simbad", "vizier", "openngc", "mast"},
		hasField:   func(t Target) bool { return !t.Epoch.IsZero() },
		take: func(dst *Target, src Target, provider string) {
			dst.Epoch = src.Epoch
			setProvenance(dst, "Epoch", provider)
		},
	},
	{
		precedence: []string{"simbad"},
		hasField:   func(t Target) bool { return t.RadialVelocity != 0 },
		take: func(dst *Target, src Target, provider string) {
			dst.RadialVelocity = src.RadialVelocity
			setProvenance(dst, "RadialVelocity", provider)
		},
	},
	{
		// SBDB-only physical-parameter cluster (H/G/M1/K1/M2/K2/G1/G2) —
		// no other provider populates any of these today.
		precedence: []string{"sbdb"},
		hasField:   func(t Target) bool { return t.HasH || t.HasM1 || t.HasG1G2 },
		take: func(dst *Target, src Target, provider string) {
			dst.H, dst.HasH = src.H, src.HasH
			dst.G = src.G
			dst.M1, dst.HasM1 = src.M1, src.HasM1
			dst.K1 = src.K1
			dst.M2 = src.M2
			dst.K2 = src.K2
			dst.G1, dst.G2, dst.HasG1G2 = src.G1, src.G2, src.HasG1G2
			setProvenance(dst, "PhysicalParams", provider)
		},
	},
}

// astrometricPrecedence orders providers for Coord/Parallax/PmRA/PmDec,
// treated as one coupled cluster: mixing Coord from one source with
// proper motion from another would describe an internally-inconsistent
// astrometric solution, so whichever provider wins Coord also supplies
// whatever of Parallax/PmRA/PmDec it has (possibly none).
var astrometricPrecedence = []string{"gaia", "simbad", "openngc", "mast", "vizier"}

// mergeGroup coalesces g's candidates into one Target: identity/metadata
// fields take the first group member (registration order) with a value;
// Aliases is always a union; Coord/Parallax/PmRA/PmDec come from the
// highest-precedence trustworthy-coord member; every other field follows
// scalarFieldRules.
func mergeGroup(g group) Target {
	var merged Target

	for _, c := range g.candidates {
		t := c.target

		if merged.ID == "" && t.ID != "" {
			merged.ID = t.ID
			setProvenance(&merged, "ID", c.provider)
		}

		if merged.Name == "" && t.Name != "" {
			merged.Name = t.Name
			setProvenance(&merged, "Name", c.provider)
		}

		if merged.Designation == "" && t.Designation != "" {
			merged.Designation = t.Designation
			setProvenance(&merged, "Designation", c.provider)
		}

		if merged.SPKID == "" && t.SPKID != "" {
			merged.SPKID = t.SPKID
			setProvenance(&merged, "SPKID", c.provider)
		}

		if merged.Catalog == "" && t.Catalog != "" {
			merged.Catalog = t.Catalog
			setProvenance(&merged, "Catalog", c.provider)
		}

		if merged.Kind == "" && t.Kind != "" {
			merged.Kind = t.Kind
			setProvenance(&merged, "Kind", c.provider)
		}

		if merged.TLELine1 == "" && t.TLELine1 != "" {
			merged.TLELine1 = t.TLELine1
			setProvenance(&merged, "TLELine1", c.provider)
		}

		if merged.TLELine2 == "" && t.TLELine2 != "" {
			merged.TLELine2 = t.TLELine2
			setProvenance(&merged, "TLELine2", c.provider)
		}
	}

	merged.Aliases = collectAliases(g)

astrometry:
	for _, provider := range astrometricPrecedence {
		for _, c := range g.candidates {
			if c.provider != provider || !trustworthyCoord(c.target, c.provider) {
				continue
			}

			merged.Coord, merged.HasCoord = c.target.Coord, true
			setProvenance(&merged, "Coord", provider)

			if c.target.Parallax != 0 {
				merged.Parallax = c.target.Parallax
				setProvenance(&merged, "Parallax", provider)
			}

			if c.target.PmRA != 0 {
				merged.PmRA = c.target.PmRA
				setProvenance(&merged, "PmRA", provider)
			}

			if c.target.PmDec != 0 {
				merged.PmDec = c.target.PmDec
				setProvenance(&merged, "PmDec", provider)
			}

			break astrometry
		}
	}

	for _, rule := range scalarFieldRules {
		for _, provider := range rule.precedence {
			for _, c := range g.candidates {
				if c.provider != provider || !rule.hasField(c.target) {
					continue
				}

				rule.take(&merged, c.target, provider)

				goto nextRule
			}
		}

	nextRule:
	}

	return merged
}

// collectAliases unions every group member's own ID and Aliases list —
// never precedence-picked, since more cross-IDs only helps a future
// alias-graph pass, and this is exactly what feeds it.
func collectAliases(g group) []string {
	seen := make(map[string]bool)

	var out []string

	add := func(v string) {
		if v == "" {
			return
		}

		key := resolve.Normalize(v)
		if seen[key] {
			return
		}

		seen[key] = true

		out = append(out, v)
	}

	for _, c := range g.candidates {
		add(c.target.ID)

		for _, a := range c.target.Aliases {
			add(a)
		}
	}

	return out
}
