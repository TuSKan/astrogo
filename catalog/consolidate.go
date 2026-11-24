package catalog

import (
	"slices"

	"github.com/TuSKan/astrogo/catalog/resolve"
)

// A Record is one provider's record of an object, as the provider returned
// it.
type Record struct {
	// Provider names the provider, as its Name method does: "simbad",
	// "openngc". The merge policy ranks providers by this name.
	Provider string
	// Target is the provider's record.
	Target Target
}

// Consolidate returns one Target for each object the records describe.
//
// Records are one object when they share a [resolve.IdentityKeys] key,
// directly or through other records. Nothing compares positions: a star and
// its companion a few arcseconds away are two objects, and so are a star and
// the nebula around it, unless an identifier says otherwise.
//
// A group is left unmerged when it would make one object of two records from
// one provider that share no key, since a provider lists each object once. A
// record naming two of another provider's objects is evidence of an
// ambiguity, not of identity: OpenNGC catalogs NGC 1990 as the star Alnilam
// and lists the star's designations, where SIMBAD keeps NGC 1990 as the
// nebula and the star as eps Ori. Each record of such a group is returned as
// it came, so an object can be listed twice but never lost.
//
// Every Target names what it was built from. Its Sources are its records'
// [resolve.SourceRecord]s. A merged Target's Associations give, as accepted
// [resolve.Association]s, the identifier that joined each record to the
// group, and the provider that asserted it. A record of an ambiguous group
// keeps its own source, with an ambiguous association to each record it was
// kept apart from, naming the identifier that would have joined them.
//
// A record that matches no other is returned as it came. A group is merged
// under the policy [Resolver.Resolve] applies to the records it cross-matches:
//   - the ID, name, designation, SPK ID, catalog and kind from the first
//     record that has each;
//   - the position with its epoch, parallax, proper motion and their errors
//     together, from the first provider with a position in the order Gaia,
//     SIMBAD, OpenNGC, MAST, VizieR, so one astrometric solution is never
//     mixed with another;
//   - each other measurement from the best-ranked provider that has it: the
//     V magnitude from SIMBAD, OpenNGC, then Gaia, whose is converted from
//     G; the radial velocity from SIMBAD; a body's physical parameters, its
//     orbital elements with their epoch, and a TLE with its epoch, each from
//     one provider;
//   - a provider the policy does not rank supplying, after those it does and
//     in the records' order, what none of them has.
//
// The merged Target's Provenance names the provider of each field. Its
// aliases are every record's ID and aliases; no record's display Name is
// added, since an alias is an identity key and a display name is not. What it
// is a member of, MemberOf, is every record's.
//
// Targets come in the order of their first records.
func Consolidate(records []Record) []Target {
	cands := make([]candidate, len(records))
	for i, r := range records {
		cands[i] = candidate{target: r.Target, provider: r.Provider}
	}

	uf := newUnionFind(len(cands))
	links := unionByAlias(cands, uf)

	members := make(map[int][]int, len(cands))
	for i := range cands {
		root := uf.find(i)
		members[root] = append(members[root], i)
	}

	out := make([]Target, 0, len(cands))

	for i, c := range cands {
		m := members[uf.find(i)]

		switch {
		case len(m) == 1:
			t := c.target
			t.Sources = []resolve.SourceRecord{sourceOf(c)}
			out = append(out, t)
		case ambiguous(cands, m):
			t := c.target
			t.Sources = []resolve.SourceRecord{sourceOf(c)}

			for _, l := range links {
				if l.a == i || l.b == i {
					t.Associations = append(t.Associations, associationOf(cands, l, resolve.Ambiguous))
				}
			}

			out = append(out, t)
		case m[0] == i:
			g := group{candidates: make([]candidate, len(m))}
			for k, j := range m {
				g.candidates[k] = cands[j]
			}

			for _, l := range links {
				if uf.find(l.a) == uf.find(i) {
					g.associations = append(g.associations, associationOf(cands, l, resolve.Accepted))
				}
			}

			out = append(out, mergeGroup(g))
		}
	}

	return out
}

// ambiguous reports whether the group of cands at indices m holds two
// records from one provider that share no identity key: one provider's two
// objects, joined only through another provider's record.
func ambiguous(cands []candidate, m []int) bool {
	keys := make([][]string, len(m))
	for k, j := range m {
		keys[k] = resolve.IdentityKeys(cands[j].target)
	}

	for a := range m {
		for b := a + 1; b < len(m); b++ {
			if cands[m[a]].provider != cands[m[b]].provider {
				continue
			}

			if !slices.ContainsFunc(keys[a], func(k string) bool { return slices.Contains(keys[b], k) }) {
				return true
			}
		}
	}

	return false
}
