// Package xmatch cross-matches astronomical catalog entries.
//
// It identifies resolve.Target values from independently-obtained sources that
// describe the same physical object, as a standalone, directly-testable
// primitive operating on plain resolve.Target slices.
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
// caller. A caller propagating a Pair's Target has to attach the Target's
// own PmRA, PmDec, Parallax and RadialVelocity to its Coord first, as
// coord.NewICRSWithKinematics does: providers keep the motion in those
// fields, and coord.PropagateEpoch on a bare Coord moves nothing (#726).
package xmatch

import (
	"sort"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/internal/posmatch"
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

// WithPositionMatchThreshold sets the maximum angular separation, at a
// common epoch, at which two Targets sharing no alias or ID are still
// considered the same object. See Match for which epoch that is.
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
//  2. Positional fallback: any Target whose group has no member in the
//     other slice after pass 1 (skipped entirely for a Target with
//     !HasCoord or an all-zero Coord) is compared with every such
//     candidate in the other slice at a common epoch, carrying each
//     Target's own proper motion: J2000 when both move, the unmoving one's
//     own epoch when only one does, since a row that publishes no motion
//     cannot be moved, and as given when neither does. Pairs within
//     WithPositionMatchThreshold are accepted closest first, each group
//     pairing at most once, so the result does not depend on the order of
//     either slice.
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

// unpairedIndices returns the indices whose union-find group has no member
// from the other slice yet — the only candidates eligible for positional
// matching, since a group the alias pass already paired needs no further
// check.
//
// Not "groups of one": a row that appears twice in one slice is grouped with
// its copy by the alias pass and is still unpaired. Until #728 that pair of
// copies was skipped here, so duplicating a row lost its positional match.
func unpairedIndices(items []item, uf *unionFind) []int {
	var origins [2]map[int]bool

	origins[0], origins[1] = make(map[int]bool), make(map[int]bool)

	for i, it := range items {
		origins[it.origin][uf.find(i)] = true
	}

	out := make([]int, 0, len(items))

	for i := range items {
		if root := uf.find(i); !origins[0][root] || !origins[1][root] {
			out = append(out, i)
		}
	}

	return out
}

// unionByPosition pairs still-unpaired, trustworthy-coord items across the
// two slices by position, compared at a common epoch (posmatch.Separation),
// closest pair first: every cross-slice pair within threshold is a
// candidate, and a candidate is accepted unless either side's group has
// already been paired. Each group is therefore paired with at most one group
// from the other slice, and the result does not depend on input order.
//
// Until #728 each item in turn took its nearest partner not yet taken, so an
// early item could claim a partner far closer to a later one, and Match(a, b)
// and Match(b, a) could pair different rows.
//
// O(M² log M) over the remaining items M: appropriate at the scale this
// package targets (two candidate lists from a handful of catalogs, not a
// bulk all-sky cross-match), the same reasoning catalog.go's own
// unionByPosition documents.
func unionByPosition(items []item, uf *unionFind, threshold angle.Angle) {
	// Each position carries its Target's own motion, which providers keep in
	// the Target's fields rather than in Coord; see posmatch.Coord (#726).
	type positioned struct {
		idx   int
		c     coord.ICRS
		epoch time.Time
	}

	var side [2][]positioned

	for _, i := range unpairedIndices(items, uf) {
		t := items[i].target
		if !trustworthyCoord(t) {
			continue
		}

		o := items[i].origin
		side[o] = append(side[o], positioned{idx: i, c: posmatch.Coord(t), epoch: t.Epoch})
	}

	type candidate struct {
		a, b int // item indices, origin 0 and origin 1
		sep  angle.Angle
	}

	var candidates []candidate

	for _, pa := range side[0] {
		for _, pb := range side[1] {
			sep, err := posmatch.Separation(pa.c, pa.epoch, pb.c, pb.epoch)
			if err != nil || sep > threshold {
				continue
			}

			candidates = append(candidates, candidate{a: pa.idx, b: pb.idx, sep: sep})
		}
	}

	// Closest first; equal separations fall back to input order so the
	// result is deterministic.
	sort.Slice(candidates, func(i, j int) bool {
		ci, cj := candidates[i], candidates[j]
		if ci.sep != cj.sep {
			return ci.sep < cj.sep
		}

		if ci.a != cj.a {
			return ci.a < cj.a
		}

		return ci.b < cj.b
	})

	paired := make(map[int]bool)

	for _, c := range candidates {
		ra, rb := uf.find(c.a), uf.find(c.b)
		if paired[ra] || paired[rb] {
			continue
		}

		uf.union(c.a, c.b)

		paired[ra], paired[rb] = true, true
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
