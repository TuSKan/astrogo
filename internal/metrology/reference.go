package metrology

import "fmt"

// ReferenceKind classifies what a suite compared against, because the kinds
// do not carry equal weight.
//
// An authoritative standard or a published table settles a question. Another
// implementation only shows that two programs agree, which is worth knowing
// and is not the same thing — particularly when the two share ancestry, for
// which see [Reference.SharedAncestor].
type ReferenceKind string

// The reference kinds astrogo actually validates against.
const (
	// KindSOFA is the IAU Standards of Fundamental Astronomy, reached here
	// through gofa.
	KindSOFA ReferenceKind = "SOFA"

	// KindIERS is Earth orientation and time data from the IERS.
	KindIERS ReferenceKind = "IERS"

	// KindSPICE is NAIF SPICE kernels and their documented conventions.
	KindSPICE ReferenceKind = "SPICE"

	// KindHorizons is JPL's Horizons on-line ephemeris service.
	KindHorizons ReferenceKind = "JPL_HORIZONS"

	// KindUSNO is the US Naval Observatory's published data services.
	KindUSNO ReferenceKind = "USNO"

	// KindNASA covers NASA/GSFC catalogs such as the Five Millennium
	// eclipse canons.
	KindNASA ReferenceKind = "NASA"

	// KindPaper is a value or table taken from peer-reviewed literature.
	KindPaper ReferenceKind = "PAPER"

	// KindImplementation is another astronomy library. Useful for finding
	// disagreements, insufficient on its own for establishing correctness.
	KindImplementation ReferenceKind = "IMPLEMENTATION"

	// KindInvariant is a mathematical or physical identity that needs no
	// external oracle — a round trip, a norm, a sign. Weaker than an
	// authority about absolute values and stronger than any of them about
	// self-consistency, since two implementations can share a mistake but
	// cannot both satisfy an identity while getting it wrong.
	KindInvariant ReferenceKind = "INVARIANT"
)

// Reference identifies what a suite was measured against, precisely enough
// that the comparison can be repeated after the service has changed.
//
// Version and Dataset matter more than they look: "validated against
// Horizons" ages into a claim nobody can check, while "validated against
// Horizons with DE440, kernel SHA-256 abc..., on 2026-08-28" stays meaningful
// after both sides have moved on.
type Reference struct {
	// Kind is how much weight the comparison carries. See [ReferenceKind].
	Kind ReferenceKind

	// Name is the reference as a person would cite it, e.g. "JPL Horizons"
	// or "Masana et al. 2021 (GAMBONS)".
	Name string

	// Version pins the reference: a release tag, a data vintage, a paper
	// year. Empty when the reference genuinely has no version, such as an
	// invariant.
	Version string

	// Source locates it — a URL, a DOI, a routine name.
	Source string

	// Dataset names the specific data used, where the reference serves more
	// than one: a kernel filename, a catalogue release, an airglow file.
	Dataset string

	// SharedAncestor names the common origin when this reference and
	// astrogo's own implementation descend from the same source, and is
	// empty when they are genuinely independent.
	//
	// This is the field that keeps a report honest. astrogo computes IAU
	// reductions through gofa, which is SOFA-derived; Astropy computes them
	// through ERFA, which is also SOFA-derived. Agreement between them says
	// the two translations of SOFA are faithful, which is worth testing and
	// is not evidence that SOFA's model is right or that astrogo applies it
	// in the right order. Setting this to "SOFA" makes a generated report
	// label the row rather than leaving a reader to already know.
	SharedAncestor string
}

// Independent reports whether the reference shares no known ancestry with
// astrogo's own implementation of the quantity being compared.
func (r Reference) Independent() bool { return r.SharedAncestor == "" }

// Provenance renders the reference as a single citable line, including the
// shared-ancestry caveat when there is one.
func (r Reference) Provenance() string {
	s := string(r.Kind) + " " + r.Name

	if r.Version != "" {
		s += " " + r.Version
	}

	if r.Dataset != "" {
		s += " [" + r.Dataset + "]"
	}

	if r.Source != "" {
		s += " (" + r.Source + ")"
	}

	if !r.Independent() {
		s += fmt.Sprintf(" — consistency check: shares %s ancestry with astrogo, not independent validation",
			r.SharedAncestor)
	}

	return s
}
