package posmatch

import (
	"math"
	"slices"
	"sort"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

// CandidatePairs returns, in order, the pairs (i, j) for which
// Compare(a[i], b[j], resolution) may find a[i] and b[j] compatible: every
// pair it does, and otherwise only pairs whose positions lie within what
// their errors, their motion and resolution allow. It finds them through
// declination-sorted indexes, so its cost grows with the number of targets
// and of nearby pairs, not with their product.
//
// Each target is indexed where Compare can place it without knowing its
// partner: a moving target at J2000.0, where two moving targets are
// compared, and a still one as it is given. Around that place, a pair is
// sought within the larger of resolution and the two targets' error reach,
// √(11.83 · tr C) each at the most uncertain epoch they are compared at,
// since D² ≤ 11.83 puts their offset within √(11.83 · (tr C₁ + tr C₂)). A
// moving target compared with a still one is compared at the still one's
// epoch, so that search also reaches as far as the moving one travels
// between J2000.0 and the other side's still targets' epochs.
func CandidatePairs(a, b []resolve.Target, resolution angle.Angle) [][2]int {
	pa, pb := place(a), place(b)

	loA, hiA, stillInA := stillEpochs(pa)
	loB, hiB, stillInB := stillEpochs(pb)

	for i := range pa {
		pa[i].reach(loB, hiB, stillInB)
	}

	for j := range pb {
		pb[j].reach(loA, hiA, stillInA)
	}

	res := math.Max(resolution.Radians(), 0)

	movingB, stillB, stillA := newDecIndex(pb, true), newDecIndex(pb, false), newDecIndex(pa, false)

	var pairs [][2]int

	for i, p := range pa {
		if !p.ok {
			continue
		}

		if p.moving {
			// At J2000.0 with b's moving targets.
			movingB.within(p.anchor, p.radius(res, movingB), func(j int) { pairs = append(pairs, [2]int{i, j}) })
			// At each of b's still targets' epochs, which p reaches by traveling.
			if r := p.radius(res, stillB); r > 0 {
				stillB.within(p.anchor, r+p.travel, func(j int) { pairs = append(pairs, [2]int{i, j}) })
			}

			continue
		}

		// As given, with b's still targets.
		stillB.within(p.anchor, p.radius(res, stillB), func(j int) { pairs = append(pairs, [2]int{i, j}) })
	}

	// a's still targets with b's moving ones, at the still target's epoch, which
	// the moving one reaches by traveling: sought from the moving side.
	for j, q := range pb {
		if !q.ok || !q.moving {
			continue
		}

		if r := q.radius(res, stillA); r > 0 {
			stillA.within(q.anchor, r+q.travel, func(i int) { pairs = append(pairs, [2]int{i, j}) })
		}
	}

	slices.SortFunc(pairs, func(x, y [2]int) int {
		if x[0] != y[0] {
			return x[0] - y[0]
		}

		return x[1] - y[1]
	})

	return pairs
}

// placed is a target as CandidatePairs indexes it.
type placed struct {
	target resolve.Target
	// ok is false for a moving target that cannot be moved to J2000.0,
	// which Compare can pair with nothing.
	ok, moving bool
	// anchor is the moving target at J2000.0, or the still one as given.
	anchor coord.ICRS
	// epoch is a still target's own epoch, J2000.0 when it gives none.
	epoch time.Time
	// errReach is √(11.83 · tr C) in radians at the most uncertain epoch the
	// target is compared at, where hasErr says it publishes errors there.
	errReach float64
	hasErr   bool
	// travel is how far, in radians, a moving target moves from its anchor
	// over the other side's still targets' epochs.
	travel float64
}

// place places each target where CandidatePairs indexes it.
func place(ts []resolve.Target) []placed {
	out := make([]placed, len(ts))

	for i, t := range ts {
		p := placed{target: t, epoch: t.Epoch, ok: true}
		if p.epoch.IsZero() {
			p.epoch = time.J2000()
		}

		c := Coord(t)
		p.moving = moving(c)
		p.anchor = c

		if p.moving {
			at, err := coord.PropagateEpoch(c, time.J2000())
			p.anchor, p.ok = at, err == nil
		}

		out[i] = p
	}

	return out
}

// stillEpochs returns the earliest and latest epoch of ps's still targets,
// and false when there are none.
func stillEpochs(ps []placed) (lo, hi time.Time, ok bool) {
	for _, p := range ps {
		if !p.ok || p.moving {
			continue
		}

		if !ok || p.epoch.Before(lo) {
			lo = p.epoch
		}

		if !ok || p.epoch.After(hi) {
			hi = p.epoch
		}

		ok = true
	}

	return lo, hi, ok
}

// reach sets p's error reach and travel for comparisons with a side whose
// still targets' epochs run from lo to hi, when stillThere says it has any.
//
// A still target is compared at its own epoch, whatever the other one does. A
// moving one is compared at J2000.0 with a moving target, and at each still
// target's epoch with a still one. Where it publishes motion errors, its
// covariance trace is a convex quadratic in time, so over those epochs it is
// greatest at J2000.0, lo or hi. Where it publishes none, it has a
// covariance only at the epoch its errors hold at, which is one of those
// epochs when it lies between lo and hi. Its distance from its J2000.0 place
// grows with the time from J2000.0, so it too is greatest at lo or hi.
func (p *placed) reach(lo, hi time.Time, stillThere bool) {
	if !p.ok {
		return
	}

	epochs := []time.Time{p.epoch}
	if p.moving {
		epochs = []time.Time{time.J2000()}
		if stillThere {
			epochs = append(epochs, lo, hi)

			if e := errorEpoch(p.target); !e.Before(lo) && !e.After(hi) {
				epochs = append(epochs, e)
			}
		}
	}

	var trace float64

	for _, e := range epochs {
		if c, ok := covarianceAt(p.target, e); ok {
			trace = math.Max(trace, c.xx+c.yy)
			p.hasErr = true
		}
	}

	p.errReach = math.Sqrt(maxD2 * trace)

	if !p.moving || !stillThere {
		return
	}

	for _, e := range []time.Time{lo, hi} {
		at, err := coord.PropagateEpoch(Coord(p.target), e)
		if err != nil {
			// Compare cannot move it there either, but it may move it to an
			// epoch between; searching the whole sky keeps the pair.
			p.travel = math.Pi

			return
		}

		p.travel = math.Max(p.travel, coord.Separation(p.anchor, at).Radians())
	}
}

// radius is how far, in radians, from p's anchor a partner from ix can lie and
// still be compatible with p, before any travel: within resolution res, or
// within p's error reach plus the largest of ix's where both publish errors.
// It is zero when no partner from ix can be.
func (p placed) radius(res float64, ix decIndex) float64 {
	r := res
	if p.hasErr && ix.hasErr {
		r = math.Max(r, p.errReach+ix.maxErrReach)
	}

	return r
}

// decIndex holds targets' anchors sorted by declination.
type decIndex struct {
	entries []indexed
	// maxErrReach is the largest error reach among the targets that publish
	// errors, where hasErr says any does.
	maxErrReach float64
	hasErr      bool
}

type indexed struct {
	dec    float64
	i      int
	anchor coord.ICRS
}

// newDecIndex indexes ps's moving targets, or its still ones.
func newDecIndex(ps []placed, moving bool) decIndex {
	var ix decIndex

	for i, p := range ps {
		if !p.ok || p.moving != moving {
			continue
		}

		ix.entries = append(ix.entries, indexed{dec: p.anchor.Dec().Radians(), i: i, anchor: p.anchor})

		if p.hasErr {
			ix.maxErrReach = math.Max(ix.maxErrReach, p.errReach)
			ix.hasErr = true
		}
	}

	sort.Slice(ix.entries, func(x, y int) bool { return ix.entries[x].dec < ix.entries[y].dec })

	return ix
}

// within yields the index of every target within r radians of c, scanning
// only the band of declinations r allows. A radius of zero or less yields
// none.
//
// r is widened by a picoradian so that a pair exactly at its bound, as the
// separation's rounding may put it, is never lost.
func (ix decIndex) within(c coord.ICRS, r float64, yield func(int)) {
	if r <= 0 {
		return
	}

	r += 1e-12
	dec := c.Dec().Radians()

	k := sort.Search(len(ix.entries), func(k int) bool { return ix.entries[k].dec >= dec-r })
	for ; k < len(ix.entries) && ix.entries[k].dec <= dec+r; k++ {
		if coord.Separation(c, ix.entries[k].anchor).Radians() <= r {
			yield(ix.entries[k].i)
		}
	}
}
