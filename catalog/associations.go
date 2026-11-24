package catalog

import (
	"regexp"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
)

// link is one join the cross-match made: two candidates, and what joined
// them.
type link struct {
	a, b int
	// key is the identity key both carry, or empty for a positional join.
	key string
	// sep is the separation of a positional join.
	sep angle.Angle
	// d2 is a positional join's D², where hasD2 says it was computed.
	d2    float64
	hasD2 bool
}

// releasePattern finds a data release in a provider's Catalog: "DR3" in
// "Gaia DR3".
var releasePattern = regexp.MustCompile(`\bE?DR\d+\b`)

// sourceOf is the source record of one candidate: its provider, the release
// its Catalog names, and its own ID.
func sourceOf(c candidate) resolve.SourceRecord {
	return resolve.SourceRecord{Provider: c.provider, Release: releasePattern.FindString(c.target.Catalog), ID: c.target.ID}
}

// sourcesOf is the source record of every candidate, in order.
func sourcesOf(cs []candidate) []resolve.SourceRecord {
	out := make([]resolve.SourceRecord, len(cs))
	for i, c := range cs {
		out[i] = sourceOf(c)
	}

	return out
}

// associationOf states the evidence of one link between candidates.
func associationOf(cands []candidate, l link, status resolve.AssociationStatus) resolve.Association {
	a, b := cands[l.a], cands[l.b]
	as := resolve.Association{
		From:         sourceOf(a),
		To:           sourceOf(b),
		Relationship: resolve.SameObject,
		Status:       status,
	}

	if l.key == "" {
		as.Separation = l.sep
		as.D2, as.HasD2 = l.d2, l.hasD2
		as.Authority = "astrogo"

		return as
	}

	as.Evidence = identifierFor(a.target, l.key)

	// The record that lists the identifier among others' designations is
	// the one asserting the cross-identification.
	ownA := resolve.ParseIdentifier(a.target.ID) == as.Evidence
	ownB := resolve.ParseIdentifier(b.target.ID) == as.Evidence

	as.Authority = a.provider
	if ownA && !ownB {
		as.Authority = b.provider
	}

	return as
}

// identifierFor is the identifier of t whose identity key is key.
func identifierFor(t Target, key string) resolve.Identifier {
	keys, ids := resolve.IdentityKeys(t), resolve.Identifiers(t)
	for i, k := range keys {
		if k == key {
			return ids[i]
		}
	}

	return resolve.Identifier{}
}
