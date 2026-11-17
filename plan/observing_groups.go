package plan

import (
	"cmp"
	"math"
	"slices"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
)

// eyeResolution is the separation the naked eye resolves: one minute of arc,
// the detail a letter on Snellen's chart subtends at the distance 20/20 (6/6)
// vision reads it. Two stars closer together than that are one point of light
// to the eye.
var eyeResolution = angle.Arcmin(1)

// WithResolution sets how far apart two stars must be for the observer to see
// them as two. VisibleTonight lists stars closer together than sep as one row,
// an observing group, with each star among its Members (see
// [VisibleObject.Members]).
//
// The default is one minute of arc, what the naked eye resolves, so alf Cen A
// and B are one row and so are alf01 and alf02 Cru. A telescope resolves far
// more: with its own limit passed here, the components it splits are rows of
// their own. Zero, or anything below it, lists every star alone.
func WithResolution(sep angle.Angle) VisibleTonightOption {
	return func(c *visibleTonightConfig) { c.resolution = sep }
}

// placedObject is a visible object together with where it is tonight, which
// grouping needs and VisibleObject does not carry: its astrometric place at
// its peak, proper motion applied.
type placedObject struct {
	pos coord.ICRS
	vo  VisibleObject
}

// observingGroups lists objs as the observer sees them, one row for each
// point of light (#1182).
//
// Only stars are grouped, KindStar and KindDoubleStar records: a nebula or a
// galaxy is not a point, and a solar-system body moves against the stars
// through the night. Two stars closer together than resolution are one point,
// and so, through them, is a chain of such stars.
//
// A group's row is its brightest counted member, with the group's combined
// magnitude, m = -2.5 log10(sum of 10^(-0.4 m_i)) over the counted members,
// and every member in Members. Which members are counted follows the
// catalog's hierarchy, [resolve.Target.MemberOf], so each star's light is
// counted once:
//   - A system record whose listed components are all in its group already
//     holds their light, so it is counted and they are not: alf Cen's V -0.1
//     is A's and B's together.
//   - A system record with a listed component outside its group, which the
//     observer resolves, holds light another row shows, so it is not counted,
//     and its components inside the group are.
//   - Members with no hierarchy between them, alf01 and alf02 Cru, are
//     distinct stars and are each counted.
//
// A system record left with nothing counted in its group, all of its
// components resolved into rows of their own, is no point of light of its
// own: it is listed among the Members of its brightest component's row
// instead.
func observingGroups(objs []placedObject, resolution angle.Angle) []VisibleObject {
	out := make([]VisibleObject, 0, len(objs))

	if resolution <= 0 {
		for _, o := range objs {
			out = append(out, o.vo)
		}

		return out
	}

	h := newHierarchy(objs)
	root := groupByResolution(objs, resolution)

	members := make(map[int][]int, len(objs))
	for i := range objs {
		members[root[i]] = append(members[root[i]], i)
	}

	// counted[i] reports whether object i's light is counted in its group's
	// magnitude. orphans are the members of groups with nothing counted.
	counted := make([]bool, len(objs))

	var orphans []int

	for r, m := range members {
		anyCounted := false

		for _, i := range m {
			counted[i] = h.counts(i, root, r)
			anyCounted = anyCounted || counted[i]
		}

		if !anyCounted {
			orphans = append(orphans, m...)

			delete(members, r)
		}
	}

	slices.Sort(orphans)

	for _, i := range orphans {
		if host, ok := h.brightestCountedDescendant(i, objs, counted); ok {
			members[root[host]] = append(members[root[host]], i)
			continue
		}

		// Nothing to attach it to: a hierarchy this object's own data
		// contradicts. It is a row of its own rather than lost.
		counted[i] = true
		members[i] = []int{i}
	}

	// Each group is keyed by its lowest index, so its row comes where its
	// first member did.
	for i := range objs {
		if m, ok := members[i]; ok {
			out = append(out, groupRow(objs, m, counted))
		}
	}

	return out
}

// groupRow is the row of the group of objs at indices m: its brightest
// counted member, with the counted members' combined magnitude and every
// member in Members. A group of one is that object's row, unchanged.
func groupRow(objs []placedObject, m []int, counted []bool) VisibleObject {
	if len(m) == 1 {
		return objs[m[0]].vo
	}

	sorted := slices.Clone(m)
	slices.SortStableFunc(sorted, func(a, b int) int {
		return cmp.Compare(objs[a].vo.ApparentMag, objs[b].vo.ApparentMag)
	})

	var (
		flux   float64
		row    VisibleObject
		nCount int
	)

	members := make([]VisibleObject, 0, len(sorted))

	for _, i := range sorted {
		members = append(members, objs[i].vo)

		if !counted[i] {
			continue
		}

		flux += math.Pow(10, -0.4*objs[i].vo.ApparentMag)

		if nCount == 0 {
			row = objs[i].vo
		}

		nCount++
	}

	// One counted member is the group's light, its magnitude as it is
	// rather than passed through the sum and back.
	if nCount > 1 {
		row.ApparentMag = -2.5 * math.Log10(flux)
	}

	row.Members = members

	return row
}

// groupByResolution unions every two stars of objs closer together than
// resolution, and returns each object's group as the lowest index in it. An
// object that is not a star is a group of its own.
//
// The stars are swept in order of declination, so each is compared only with
// those within resolution of it in declination, whatever their right
// ascension: two stars that close on the sky are that close in declination.
func groupByResolution(objs []placedObject, resolution angle.Angle) []int {
	parent := make([]int, len(objs))
	for i := range parent {
		parent[i] = i
	}

	find := func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}

		return i
	}

	var stars []int

	for i, o := range objs {
		if isPointSource(o.vo.Target.Kind) {
			stars = append(stars, i)
		}
	}

	slices.SortFunc(stars, func(a, b int) int {
		return cmp.Compare(objs[a].pos.Dec(), objs[b].pos.Dec())
	})

	for x, a := range stars {
		for _, b := range stars[x+1:] {
			if objs[b].pos.Dec()-objs[a].pos.Dec() >= resolution {
				break
			}

			if coord.Separation(objs[a].pos, objs[b].pos) < resolution {
				ra, rb := find(a), find(b)
				parent[max(ra, rb)] = min(ra, rb)
			}
		}
	}

	root := make([]int, len(objs))
	for i := range objs {
		root[i] = find(i)
	}

	return root
}

// isPointSource reports whether an object of kind is a point of light, which
// alone can be unresolved from another.
func isPointSource(kind resolve.Kind) bool {
	return kind == resolve.KindStar || kind == resolve.KindDoubleStar
}

// hierarchy is the catalog hierarchy among a night's stars: for each, the
// stars listed with it that it is a member of, and those that are members of
// it. Only stars take part, so a cluster a star belongs to is not its parent
// here.
type hierarchy struct {
	parents  [][]int
	children [][]int
}

// newHierarchy reads each star's MemberOf against the other stars' identity
// keys, so a parent is found under any of its identifiers.
func newHierarchy(objs []placedObject) hierarchy {
	owner := make(map[string]int)

	for i, o := range objs {
		if !isPointSource(o.vo.Target.Kind) {
			continue
		}

		for _, k := range resolve.IdentityKeys(o.vo.Target) {
			if _, taken := owner[k]; !taken {
				owner[k] = i
			}
		}
	}

	h := hierarchy{parents: make([][]int, len(objs)), children: make([][]int, len(objs))}

	for i, o := range objs {
		if !isPointSource(o.vo.Target.Kind) {
			continue
		}

		for _, name := range o.vo.Target.MemberOf {
			for _, k := range resolve.IdentityKeys(resolve.Target{ID: name}) {
				p, ok := owner[k]
				if !ok || p == i || slices.Contains(h.parents[i], p) {
					continue
				}

				h.parents[i] = append(h.parents[i], p)
				h.children[p] = append(h.children[p], i)
			}
		}
	}

	return h
}

// counts reports whether object i's light is counted in group g: i holds all
// of its own listed components, and no ancestor of i in g holds all of its
// own, i's light among them.
func (h hierarchy) counts(i int, root []int, g int) bool {
	if !h.whole(i, root, g) {
		return false
	}

	seen := map[int]bool{i: true}
	queue := slices.Clone(h.parents[i])

	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]

		if seen[p] {
			continue
		}

		seen[p] = true

		if root[p] == g && h.whole(p, root, g) {
			return false
		}

		queue = append(queue, h.parents[p]...)
	}

	return true
}

// whole reports whether every listed component of object i, at any depth, is
// in group g: then i's light is all in g.
func (h hierarchy) whole(i int, root []int, g int) bool {
	seen := map[int]bool{i: true}
	queue := slices.Clone(h.children[i])

	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]

		if seen[c] {
			continue
		}

		seen[c] = true

		if root[c] != g {
			return false
		}

		queue = append(queue, h.children[c]...)
	}

	return true
}

// brightestCountedDescendant returns the brightest of object i's listed
// components, at any depth, whose light is counted in its own group.
func (h hierarchy) brightestCountedDescendant(i int, objs []placedObject, counted []bool) (int, bool) {
	best, found := 0, false
	seen := map[int]bool{i: true}
	queue := slices.Clone(h.children[i])

	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]

		if seen[c] {
			continue
		}

		seen[c] = true

		if counted[c] && (!found || objs[c].vo.ApparentMag < objs[best].vo.ApparentMag) {
			best, found = c, true
		}

		queue = append(queue, h.children[c]...)
	}

	return best, found
}
