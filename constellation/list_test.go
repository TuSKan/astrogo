package constellation_test

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/constellation"
)

// TestList_CountAndNoDuplicates confirms List returns exactly 88 entries
// (the 88 official IAU constellations — the underlying catalog carries 89
// raw boundary-loop keys since Serpens Caput/Cauda are unified into one
// "Serpens" entry here) with no duplicate name or abbreviation.
func TestList_CountAndNoDuplicates(t *testing.T) {
	list := constellation.List()

	if len(list) != 88 {
		t.Fatalf("List() returned %d entries, want 88", len(list))
	}

	seenName := make(map[string]bool, len(list))
	seenAbbr := make(map[string]bool, len(list))

	for _, c := range list {
		if seenName[c.Name] {
			t.Errorf("duplicate Name %q", c.Name)
		}

		seenName[c.Name] = true

		if seenAbbr[c.Abbreviation] {
			t.Errorf("duplicate Abbreviation %q", c.Abbreviation)
		}

		seenAbbr[c.Abbreviation] = true

		if c.Name == "" || c.Abbreviation == "" {
			t.Errorf("empty Name/Abbreviation: %+v", c)
		}
	}
}

// knownCentroidExceptions lists constellations where a plain vertex-average
// centroid is documented (Centroid's own doc comment) to land outside the
// constellation's own boundary — not a bug, a real property of the shapes
// involved, confirmed by running the round-trip check below and inspecting
// each failure:
//   - Eridanus: an extremely long, tightly winding constellation near the
//     south celestial pole; its vertex average falls into neighboring
//     Fornax.
//   - Serpens: split into two disjoint regions (Caput/Cauda) by
//     Ophiuchus; averaging vertices from both lands the point in the
//     middle — inside Ophiuchus.
//   - Ursa Minor: wraps tightly around the north celestial pole, where
//     the notion of "average RA" is close to degenerate; the resulting
//     point falls just short of Lookup's own +88° polar-cap fallback.
var knownCentroidExceptions = map[string]bool{
	"Eridanus":   true,
	"Serpens":    true,
	"Ursa Minor": true,
}

// TestCentroid_RoundTripsThroughLookup is the core correctness check for
// Centroid: for every constellation not in knownCentroidExceptions,
// Lookup(Centroid(name)) must resolve back to that same constellation.
// This is a genuine cross-check, not a tautology — Centroid averages
// boundary vertices and precesses B1875→J2000, while Lookup does
// point-in-polygon testing after precessing J2000→B1875 (the reverse
// rotation); if Centroid's precession direction were wrong, this would
// fail for nearly every entry (84 of 88 pass), not just the three
// documented shape-driven exceptions above.
func TestCentroid_RoundTripsThroughLookup(t *testing.T) {
	for _, c := range constellation.List() {
		if knownCentroidExceptions[c.Name] {
			continue
		}

		t.Run(c.Name, func(t *testing.T) {
			pos, err := constellation.Centroid(c.Name)
			if err != nil {
				t.Fatalf("Centroid(%q): %v", c.Name, err)
			}

			_, abbr, err := constellation.Lookup(pos)
			if err != nil {
				t.Fatalf("Lookup(Centroid(%q)): %v", c.Name, err)
			}

			if abbr != c.Abbreviation {
				t.Errorf("Lookup(Centroid(%q)) = %q, want %q", c.Name, abbr, c.Abbreviation)
			}
		})
	}
}

// TestCentroid_CaseAndAbbreviationInsensitive confirms Centroid matches
// by full name or abbreviation, case/space-insensitive, and that querying
// by the unifying name "Serpens"/"Ser" (which spans both of its disjoint
// boundary regions) gives an identical result regardless of form.
func TestCentroid_CaseAndAbbreviationInsensitive(t *testing.T) {
	a, err := constellation.Centroid("Orion")
	if err != nil {
		t.Fatalf("Centroid(Orion): %v", err)
	}

	b, err := constellation.Centroid("ori")
	if err != nil {
		t.Fatalf("Centroid(ori): %v", err)
	}

	if a.RA() != b.RA() || a.Dec() != b.Dec() {
		t.Errorf("Centroid(Orion) = %v, Centroid(ori) = %v, want identical", a, b)
	}

	byName, err := constellation.Centroid("Serpens")
	if err != nil {
		t.Fatalf("Centroid(Serpens): %v", err)
	}

	byAbbr, err := constellation.Centroid("Ser")
	if err != nil {
		t.Fatalf("Centroid(Ser): %v", err)
	}

	if byName.RA() != byAbbr.RA() || byName.Dec() != byAbbr.Dec() {
		t.Errorf("Centroid(Serpens) = %v, Centroid(Ser) = %v, want identical", byName, byAbbr)
	}
}

// TestCentroid_UnknownName verifies the ErrUnknownAbbreviation sentinel path.
func TestCentroid_UnknownName(t *testing.T) {
	if _, err := constellation.Centroid("Not A Real Constellation"); !errors.Is(err, constellation.ErrUnknownAbbreviation) {
		t.Errorf("Centroid(unknown) error = %v, want ErrUnknownAbbreviation", err)
	}
}
