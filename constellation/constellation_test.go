package constellation_test

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constellation"
	"github.com/TuSKan/astrogo/coord"
)

// TestLookup_KnownBrightStars checks well-known bright stars (J2000
// coordinates) against their known constellations — a direct regression
// check against real reference values, not just internal consistency.
func TestLookup_KnownBrightStars(t *testing.T) {
	tests := []struct {
		name          string
		raDeg, decDeg float64
		wantAbbr      string
		wantFull      string
	}{
		{"Sirius", 101.28715, -16.71611, "CMa", "Canis Major"},
		{"Betelgeuse", 88.79294, 7.40706, "Ori", "Orion"},
		{"Rigel", 78.63446, -8.20164, "Ori", "Orion"},
		{"Vega", 279.23473, 38.78369, "Lyr", "Lyra"},
		{"Arcturus", 213.91530, 19.18241, "Boo", "Boötes"},
		{"Antares", 247.35192, -26.43200, "Sco", "Scorpius"},
		{"Regulus", 152.09296, 11.96721, "Leo", "Leo"},
		{"Aldebaran", 68.98016, 16.50930, "Tau", "Taurus"},
		{"Deneb", 310.35798, 45.28028, "Cyg", "Cygnus"},
		{"Acrux", 186.64961, -63.09909, "Cru", "Crux"},
		{"Canopus", 95.98787, -52.69566, "Car", "Carina"},
		{"Achernar", 24.42875, -57.23667, "Eri", "Eridanus"},
		{"Fomalhaut", 344.41269, -29.62224, "PsA", "Piscis Austrinus"},
		{"Spica", 201.29825, -11.16132, "Vir", "Virgo"},
		{"Capella", 79.17232, 45.99799, "Aur", "Auriga"},
		{"Procyon", 114.82550, 5.22499, "CMi", "Canis Minor"},
		{"Pollux", 116.32895, 28.02620, "Gem", "Gemini"},
		{"Altair", 297.69582, 8.86832, "Aql", "Aquila"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := coord.NewICRS(angle.Deg(tt.raDeg), angle.Deg(tt.decDeg))

			full, abbr, err := constellation.Lookup(pos)
			if err != nil {
				t.Fatalf("Lookup(%s) unexpected error: %v", tt.name, err)
			}

			if abbr != tt.wantAbbr || full != tt.wantFull {
				t.Errorf("Lookup(%s) = (%q, %q), want (%q, %q)", tt.name, full, abbr, tt.wantFull, tt.wantAbbr)
			}
		})
	}
}

// TestLookup_SerpensSplitRegions confirms both disconnected regions of
// Serpens (Caput and Cauda, split by Ophiuchus) resolve to the same
// "Serpens"/"Ser" name — a real quirk of the official boundary catalog,
// not a bug if both directions land on Ser.
func TestLookup_SerpensSplitRegions(t *testing.T) {
	// Unukalhai (Alpha Serpentis), in Serpens Caput.
	caput := coord.NewICRS(angle.Deg(236.06699), angle.Deg(6.42560))
	// A confirmed-interior point of the Serpens Cauda boundary region.
	cauda := coord.NewICRS(angle.Deg(270.0), angle.Deg(-2.0))

	for _, tc := range []struct {
		name string
		pos  coord.ICRS
	}{
		{"Caput", caput},
		{"Cauda", cauda},
	} {
		full, abbr, err := constellation.Lookup(tc.pos)
		if err != nil {
			t.Fatalf("Lookup(Serpens %s) unexpected error: %v", tc.name, err)
		}

		if abbr != "Ser" || full != "Serpens" {
			t.Errorf("Lookup(Serpens %s) = (%q, %q), want (\"Serpens\", \"Ser\")", tc.name, full, abbr)
		}
	}
}

// TestLookup_PoleRegions covers the two pole edge cases: Octans genuinely
// encloses the south celestial pole in the source catalog, while Ursa
// Minor's boundary tops out at Dec +88° — the small remaining north-polar
// cap has no explicit boundary and falls back to Ursa Minor by convention
// (see Lookup's doc comment).
func TestLookup_PoleRegions(t *testing.T) {
	tests := []struct {
		name          string
		raDeg, decDeg float64
		wantAbbr      string
	}{
		{"south pole", 0, -90, "Oct"},
		{"south pole, other RA", 180, -90, "Oct"},
		{"north pole", 0, 90, "UMi"},
		{"north pole, other RA", 180, 90, "UMi"},
		{"Polaris (near north pole)", 37.95456, 89.26410, "UMi"},
		{"Dec +89 fallback region", 100, 89, "UMi"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := coord.NewICRS(angle.Deg(tt.raDeg), angle.Deg(tt.decDeg))

			_, abbr, err := constellation.Lookup(pos)
			if err != nil {
				t.Fatalf("Lookup(%s) unexpected error: %v", tt.name, err)
			}

			if abbr != tt.wantAbbr {
				t.Errorf("Lookup(%s) abbr = %q, want %q", tt.name, abbr, tt.wantAbbr)
			}
		})
	}
}

// TestLookup_RASeamWraparound confirms boundary polygons that straddle the
// RA=0h/24h seam are tested correctly — the classic angle-wrap edge case.
// Alpheratz sits right at the Andromeda/Pegasus border near RA≈2°;
// Algenib (Pegasus) sits just west of the seam near RA≈3°, and a point
// deep in Pisces (which straddles the seam itself) confirms both sides.
func TestLookup_RASeamWraparound(t *testing.T) {
	tests := []struct {
		name          string
		raDeg, decDeg float64
		wantAbbr      string
	}{
		{"Alpheratz (And, RA~2deg)", 2.09702, 29.09043, "And"},
		{"Algenib (Peg, RA~3deg)", 3.30876, 15.18342, "Peg"},
		{"Pisces point near RA=0", 0.5, 5, "Psc"},
		{"Pisces point near RA=359", 359.5, 5, "Psc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos := coord.NewICRS(angle.Deg(tt.raDeg), angle.Deg(tt.decDeg))

			_, abbr, err := constellation.Lookup(pos)
			if err != nil {
				t.Fatalf("Lookup(%s) unexpected error: %v", tt.name, err)
			}

			if abbr != tt.wantAbbr {
				t.Errorf("Lookup(%s) abbr = %q, want %q", tt.name, abbr, tt.wantAbbr)
			}
		})
	}
}
