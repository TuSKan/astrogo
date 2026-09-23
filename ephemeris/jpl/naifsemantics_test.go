package jpl_test

import (
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
)

// TestNAIFMappingIsTheDocumentedMixOfCentersAndBarycenters pins which kind of
// point each body identifier means.
//
// The mapping holds two kinds of point at once: 199, 299 and 399 are body
// centers, while 4, 5, 6, 7 and 8 are system barycenters. That is not a
// mistake — a planetary kernel contains only barycenters for the giant
// planets, because their satellite systems live in separate kernels — but it
// is invisible from the identifier, and it is worth tens of milliarcseconds.
//
// Measured against Horizons' body-center commands over 2026: 0.0497 arcsec at
// Uranus, 0.0324 at Jupiter, 0.0288 at Saturn, 0.0093 at Neptune, and zero at
// Mars. Far inside every tolerance astrogo publishes, and far outside what
// someone comparing Jupiter against Horizons' default `599` expects — it reads
// as an astrogo error and is not one (#253).
//
// This is a documentation guard as much as a behavioral one. The doc comments
// on core.ID and NAIFFor state these numbers; if the mapping moves, they
// become wrong silently, and a wrong statement about which point a coordinate
// refers to is worse than no statement.
func TestNAIFMappingIsTheDocumentedMixOfCentersAndBarycenters(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		body       core.ID
		naif       int
		barycenter bool
	}{
		{core.Sun, 10, false},
		{core.Moon, 301, false},
		{core.Mercury, 199, false},
		{core.Venus, 299, false},
		{core.Earth, 399, false},

		{core.Mars, 4, true},
		{core.Jupiter, 5, true},
		{core.Saturn, 6, true},
		{core.Uranus, 7, true},
		{core.Neptune, 8, true},
	} {
		t.Run(tc.body.String(), func(t *testing.T) {
			t.Parallel()

			got, ok := jpl.NAIFFor(tc.body)
			if !ok {
				t.Fatalf("NAIFFor(%s) reports no mapping", tc.body)
			}

			if got != tc.naif {
				t.Errorf("NAIFFor(%s) = %d, want %d; the doc comments on core.ID and "+
					"NAIFFor name this number and would now be wrong", tc.body, got, tc.naif)
			}

			// NAIF's own convention: a bare 1-9 is a system barycenter, and
			// the body center is that number times 100 plus the same digit.
			// So the kind of point is readable from the identifier itself,
			// which is what makes this checkable rather than a matter of
			// remembering.
			if isBarycenter := got < 10; isBarycenter != tc.barycenter {
				kind := map[bool]string{true: "a system barycenter", false: "a body center"}

				t.Errorf("NAIFFor(%s) = %d, which is %s; the documentation says %s",
					tc.body, got, kind[isBarycenter], kind[tc.barycenter])
			}
		})
	}
}

// TestPlutoHasNoNAIFMapping pins the one body that is absent, because absent
// is easy to mistake for an oversight.
//
// A kernel-backed provider cannot serve Pluto, so eph.Default answers with a
// Kepler two-body propagation — worth 0.138 AU at worst, which is four orders
// of magnitude looser than anything else in this mapping. Anyone adding a
// NAIF number here should know they are also changing which of those two
// answers a caller gets.
func TestPlutoHasNoNAIFMapping(t *testing.T) {
	t.Parallel()

	if naif, ok := jpl.NAIFFor(core.Pluto); ok {
		t.Errorf("NAIFFor(Pluto) = %d, but this package has no Pluto kernel mapping; "+
			"adding one changes eph.Default from a Kepler propagation to an SPK lookup "+
			"and the 0.138 AU contract in docs/VALIDATION.md with it", naif)
	}
}
