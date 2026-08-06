// Package xmatch cross-matches astronomical catalog entries — identifying
// resolve.Target values from independently-obtained sources that describe
// the same physical object — as a standalone, directly-testable primitive
// operating on plain resolve.Target slices.
//
// catalog.Resolver already has its own cross-match logic
// (catalog/catalog.go's unionFind/unionByAlias/unionByPosition/mergeGroup),
// but it is tightly coupled to one query's own candidate list and its
// internal candidate/group types, and it also reconciles matched entries
// into one merged Target. This package solves the same matching problem —
// alias-graph union-find, with an epoch-normalized positional fallback for
// entries sharing no alias or ID — as a reusable capability for matching
// two Target slices obtained any way a caller likes (two bulk catalog
// exports, a survey cross-match, two Resolver.Search results), without
// going through a Resolver at all. It deliberately does not merge fields:
// Pair reports which Targets matched and leaves field reconciliation (and
// any epoch propagation a caller wants for its own purposes) to the
// caller — coord.PropagateEpoch composes directly with Pair's Targets for
// that.
package xmatch

import (
	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// Pair is one cross-matched pair of Targets, one from each of Match's two
// input slices.
type Pair struct {
	A, B resolve.Target
}

type config struct {
	positionMatchThreshold angle.Angle
}

// Option configures Match.
type Option func(*config)

// defaultPositionMatchThresholdArcsec mirrors catalog.Resolver's own
// default (catalog/catalog.go's defaultPositionMatchThreshold) — generous
// against typical SIMBAD/OpenNGC/MAST-relayed position precision, tight
// against typical inter-object separation in dense fields.
const defaultPositionMatchThresholdArcsec = 2.0

// WithPositionMatchThreshold sets the maximum angular separation (after
// epoch normalization to time.J2000 via coord.PropagateEpoch) at which two
// Targets sharing no alias or ID are still considered the same object.
// Default: 2 arcsec.
func WithPositionMatchThreshold(threshold angle.Angle) Option {
	return func(c *config) { c.positionMatchThreshold = threshold }
}

func newConfig(opts ...Option) config {
	c := config{positionMatchThreshold: angle.Arcsec(defaultPositionMatchThresholdArcsec)}
	for _, opt := range opts {
		opt(&c)
	}

	return c
}

// item is one Target from either input slice, tagged with which slice it
// came from (0 = a, 1 = b) so a matched connected component can be split
// back into cross-slice Pairs.
type item struct {
	target resolve.Target
	origin int
}

// Match cross-matches a against b and returns one Pair per matched pair of
// Targets. Matching runs in two passes, mirroring catalog.Resolver's own
// two-signal design:
//
//  1. Alias-graph: any Target in a and any Target in b sharing a
//     resolve.Normalize-d ID or Aliases entry are matched.
//  2. Positional fallback: any Target left unmatched (a singleton) after
//     pass 1 is epoch-normalized (via coord.PropagateEpoch to time.J2000;
//     skipped entirely for a Target with !HasCoord or an all-zero Coord)
//     and unioned with any still-unmatched candidate in the other slice
//     within WithPositionMatchThreshold.
//
// A Target matching more than one candidate via the alias pass (e.g. a
// duplicate row within one input slice) produces one Pair per cross-slice
// combination in its matched group. A Target present in only one slice,
// or matching nothing in the other slice, produces no Pair.
func Match(a, b []resolve.Target, opts ...Option) []Pair {
	cfg := newConfig(opts...)

	items := make([]item, 0, len(a)+len(b))
	for _, t := range a {
		items = append(items, item{target: t, origin: 0})
	}

	for _, t := range b {
		items = append(items, item{target: t, origin: 1})
	}

	uf := newUnionFind(len(items))
	unionByAlias(items, uf)
	unionByPosition(items, uf, cfg.positionMatchThreshold)

	return pairsFromGroups(items, uf)
}

// ── Alias-graph cross-match (primary signal) ──────────────────────────────

// unionByAlias indexes every item's resolve.Normalize-d ID and Aliases
// entries, then unions every pair of items sharing a bucket — the direct
// generalization of "join on shared catalog ID" to an arbitrary alias list.
func unionByAlias(items []item, uf *unionFind) {
	buckets := make(map[string][]int)

	add := func(key string, idx int) {
		if key == "" {
			return
		}

		buckets[key] = append(buckets[key], idx)
	}

	for i, it := range items {
		add(resolve.Normalize(it.target.ID), i)

		for _, alias := range it.target.Aliases {
			add(resolve.Normalize(alias), i)
		}
	}

	for _, indices := range buckets {
		for i := 1; i < len(indices); i++ {
			uf.union(indices[0], indices[i])
		}
	}
}

// ── Positional fallback cross-match (secondary signal) ────────────────────

// trustworthyCoord reports whether t.Coord should be trusted for
// positional cross-matching — HasCoord alone is not sufficient defense: a
// zero-valued ICRS reported as "present" (a known failure mode in more
// than one upstream provider before catalog.Resolver's own reconciliation
// fixes) would otherwise silently pull every such Target into one bogus
// match at (0,0).
func trustworthyCoord(t resolve.Target) bool {
	return t.HasCoord && !t.Coord.IsZero()
}

// singletonIndices returns the indices whose union-find group currently
// has size 1 — the only candidates eligible for positional matching, since
// anything the alias pass already grouped needs no further check.
func singletonIndices(items []item, uf *unionFind) []int {
	counts := make(map[int]int, len(items))
	for i := range items {
		counts[uf.find(i)]++
	}

	out := make([]int, 0, len(items))

	for i := range items {
		if counts[uf.find(i)] == 1 {
			out = append(out, i)
		}
	}

	return out
}

// unionByPosition epoch-normalizes every still-singleton, trustworthy-coord
// item to time.J2000 and unions any cross-origin pair within threshold —
// each item is unioned with at most its single nearest cross-origin match.
// O(M²) over the remaining singletons M: appropriate at the scale this
// package targets (two candidate lists from a handful of catalogs, not a
// bulk all-sky cross-match), the same reasoning catalog.go's own
// unionByPosition documents.
func unionByPosition(items []item, uf *unionFind, threshold angle.Angle) {
	singletons := singletonIndices(items, uf)

	type normalized struct {
		idx int
		c   coord.ICRS
	}

	norm := make([]normalized, 0, len(singletons))

	for _, i := range singletons {
		t := items[i].target
		if !trustworthyCoord(t) {
			continue
		}

		propagated, err := coord.PropagateEpoch(t.Coord, t.Epoch, time.J2000)
		if err != nil {
			continue
		}

		norm = append(norm, normalized{idx: i, c: propagated})
	}

	matched := make(map[int]bool, len(norm))

	for a := range norm {
		if matched[norm[a].idx] {
			continue
		}

		best := -1
		bestSep := threshold

		for b := range norm {
			if a == b || matched[norm[b].idx] {
				continue
			}

			if items[norm[a].idx].origin == items[norm[b].idx].origin {
				continue // only a cross-slice match counts as a Pair
			}

			sep := coord.Separation(norm[a].c, norm[b].c)
			if sep <= bestSep {
				best = b
				bestSep = sep
			}
		}

		if best >= 0 {
			uf.union(norm[a].idx, norm[best].idx)
			matched[norm[a].idx] = true
			matched[norm[best].idx] = true
		}
	}
}

// ── Group assembly ─────────────────────────────────────────────────────────

// pairsFromGroups walks every union-find connected component and emits one
// Pair per (origin-0, origin-1) combination found within it, preserving
// each origin slice's own relative order.
func pairsFromGroups(items []item, uf *unionFind) []Pair {
	byRoot := make(map[int][]int)

	var order []int

	for i := range items {
		root := uf.find(i)
		if _, ok := byRoot[root]; !ok {
			order = append(order, root)
		}

		byRoot[root] = append(byRoot[root], i)
	}

	var pairs []Pair

	for _, root := range order {
		indices := byRoot[root]
		if len(indices) < 2 {
			continue
		}

		var as, bs []int

		for _, i := range indices {
			if items[i].origin == 0 {
				as = append(as, i)
			} else {
				bs = append(bs, i)
			}
		}

		for _, ai := range as {
			for _, bi := range bs {
				pairs = append(pairs, Pair{A: items[ai].target, B: items[bi].target})
			}
		}
	}

	return pairs
}

// ── union-find ──────────────────────────────────────────────────────────

type unionFind struct {
	parent []int
	rank   []int
}

func newUnionFind(n int) *unionFind {
	parent := make([]int, n)
	for i := range parent {
		parent[i] = i
	}

	return &unionFind{parent: parent, rank: make([]int, n)}
}

func (uf *unionFind) find(x int) int {
	for uf.parent[x] != x {
		uf.parent[x] = uf.parent[uf.parent[x]] // path halving
		x = uf.parent[x]
	}

	return x
}

func (uf *unionFind) union(x, y int) {
	rx, ry := uf.find(x), uf.find(y)
	if rx == ry {
		return
	}

	switch {
	case uf.rank[rx] < uf.rank[ry]:
		uf.parent[rx] = ry
	case uf.rank[rx] > uf.rank[ry]:
		uf.parent[ry] = rx
	default:
		uf.parent[ry] = rx
		uf.rank[rx]++
	}
}
