package resolve

import (
	"strings"

	"github.com/TuSKan/astrogo/angle"
)

// SourceRecord identifies one provider's record of an object: who gave it,
// from which release of their data, and under what ID they keep it.
//
// The provider of a record and the namespace of an identifier are kept apart:
// SIMBAD's record of a star carries identifiers in the HD, HIP and Gaia
// namespaces, and it is still SIMBAD's record.
type SourceRecord struct {
	// Provider names the provider, as its Name method does: "simbad",
	// "gaia", "openngc".
	Provider string
	// Release is the dataset release or snapshot the record comes from,
	// "DR3" for Gaia, and empty for a provider that serves one living
	// catalog.
	Release string
	// ID is the record's ID in the provider's own data: its main identifier
	// for SIMBAD, its source ID for Gaia, its object name for OpenNGC.
	ID string
}

// Identifier is a designation read in its naming scheme: [ParseIdentifier]
// reads one the way [IdentityKeys] compares them.
type Identifier struct {
	// Namespace is the naming scheme, in lower case: "hd", "hip", "ngc",
	// "gaia"; "star" for a Bayer or Flamsteed designation, "var" for a
	// variable-star name, "name" for a common name, and "id" for an
	// identifier in no recognized scheme.
	Namespace string
	// Release is the catalog release the identifier belongs to where the
	// scheme has releases, "DR3" for a Gaia source ID, and empty elsewhere.
	Release string
	// Value is the designation within the namespace, in the one form every
	// spelling of it reads as: "37128" for "HD 037128" and "HD  37128".
	Value string
}

// ParseIdentifier reads s in its naming scheme, as [IdentityKeys] does. Two
// spellings of one designation give equal Identifiers, and a blank s gives
// the zero Identifier.
func ParseIdentifier(s string) Identifier {
	return identifierFromKey(identityKey(s))
}

// Identifiers returns t's identifiers read in their naming schemes, one for
// each of [IdentityKeys], in its order.
func Identifiers(t Target) []Identifier {
	keys := IdentityKeys(t)
	out := make([]Identifier, len(keys))

	for i, k := range keys {
		out[i] = identifierFromKey(k)
	}

	return out
}

// identifierFromKey is the Identifier an identity key encodes: its scheme,
// before the first colon, and the designation after it.
func identifierFromKey(key string) Identifier {
	if key == "" {
		return Identifier{}
	}

	ns, value, _ := strings.Cut(key, ":")
	id := Identifier{Namespace: ns, Value: value}

	if ns == "gaia" {
		id.Release, id.Value, _ = strings.Cut(value, " ")
	}

	return id
}

// Relationship is what an [Association] says of its two records.
type Relationship int

const (
	// SameObject says the two records describe one object.
	SameObject Relationship = iota + 1
)

// String names the relationship.
func (r Relationship) String() string {
	if r == SameObject {
		return "same object"
	}

	return "unknown relationship"
}

// AssociationStatus is whether an [Association] was acted on.
type AssociationStatus int

const (
	// Accepted is an association the merge acted on: the two records are
	// one Target.
	Accepted AssociationStatus = iota + 1
	// Ambiguous is an association the merge did not act on, because acting
	// on it would have made one object of two records one provider keeps
	// apart. Each record stays its own Target.
	Ambiguous
)

// String names the status.
func (s AssociationStatus) String() string {
	switch s {
	case Accepted:
		return "accepted"
	case Ambiguous:
		return "ambiguous"
	default:
		return "unknown status"
	}
}

// Association is the evidence that two source records are one object, and
// what was decided about it.
type Association struct {
	// From and To are the two records.
	From, To SourceRecord
	// Relationship is what the evidence says of them.
	Relationship Relationship
	// Evidence is the identifier both records carry, or the zero Identifier
	// for an association that rests on position alone.
	Evidence Identifier
	// Separation is the two positions' angular separation, compared at a
	// common epoch, for an association that rests on position alone, and
	// zero for one an identifier states.
	Separation angle.Angle
	// D2 is Δᵀ(C₁+C₂)⁻¹Δ at that epoch, where HasD2 says it was computed:
	// Δ is the offset between the two positions on the plane tangent at the
	// first, and C₁ and C₂ are their covariances. It is computed for an
	// association that rests on position alone between two records that
	// both publish their position errors, and two such records are
	// associated when D² is at most 11.83, the 99.73% point of a χ² with two
	// degrees of freedom. It measures whether the positions are compatible,
	// not how likely the records are to be one object. Records without it
	// are associated by Separation against the caller's threshold.
	D2    float64
	HasD2 bool
	// Authority names the provider whose record states the identifier where
	// it is not that provider's own: SIMBAD, when its record of a star lists
	// the star's HD number and HD's own record is another provider's. It is
	// the From record's provider when both state it as their own, and
	// "astrogo" for an association it made by position.
	Authority string
	// Status is whether the merge acted on it.
	Status AssociationStatus
}
