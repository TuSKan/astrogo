package plan

import (
	"errors"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constellation"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestNewConstellation_NameAndAbbreviationResolveIdentically confirms
// NewConstellation matches by full name or 3-letter abbreviation,
// case/space-insensitive, and both forms produce the same position.
func TestNewConstellation_NameAndAbbreviationResolveIdentically(t *testing.T) {
	byName, err := NewConstellation("Orion")
	if err != nil {
		t.Fatalf("NewConstellation(Orion): %v", err)
	}

	byAbbr, err := NewConstellation("ori")
	if err != nil {
		t.Fatalf("NewConstellation(ori): %v", err)
	}

	if byName.Name() != "Orion" || byName.Abbreviation() != "Ori" {
		t.Errorf("NewConstellation(Orion): Name()=%q Abbreviation()=%q, want Orion/Ori", byName.Name(), byName.Abbreviation())
	}

	tm := time.FromJD(2451545.0, time.UTC)

	posA, err := byName.Position(tm)
	if err != nil {
		t.Fatalf("Position: %v", err)
	}

	posB, err := byAbbr.Position(tm)
	if err != nil {
		t.Fatalf("Position: %v", err)
	}

	if posA.RA() != posB.RA() || posA.Dec() != posB.Dec() {
		t.Errorf("NewConstellation(Orion) position = %v, NewConstellation(ori) position = %v, want identical", posA, posB)
	}
}

// TestNewConstellation_UnknownName verifies the error path wraps
// constellation.ErrUnknownAbbreviation.
func TestNewConstellation_UnknownName(t *testing.T) {
	if _, err := NewConstellation("Not A Real Constellation"); !errors.Is(err, constellation.ErrUnknownAbbreviation) {
		t.Errorf("NewConstellation(unknown) error = %v, want ErrUnknownAbbreviation", err)
	}
}

// TestNewConstellationNamesTheConstellationAskedFor holds all 88, by name,
// by abbreviation and without spaces or case, to the constellation asked
// for (#640). Eridanus's point lies in Fornax and Serpens's in Ophiuchus,
// and those two had been named for the constellation their point fell in.
func TestNewConstellationNamesTheConstellationAskedFor(t *testing.T) {
	for _, c := range constellation.List() {
		for _, query := range []string{c.Name, c.Abbreviation, strings.ToUpper(strings.ReplaceAll(c.Name, " ", ""))} {
			got, err := NewConstellation(query)
			if err != nil {
				t.Errorf("NewConstellation(%q): %v", query, err)

				continue
			}

			if got.Name() != c.Name || got.Abbreviation() != c.Abbreviation {
				t.Errorf("NewConstellation(%q) = %s (%s), want %s (%s)", query, got.Name(), got.Abbreviation(), c.Name, c.Abbreviation)
			}
		}
	}
}

// TestNewConstellationRefusesSerpenssHalves: constellation.Centroid also
// answers to its catalog's keys for Serpens's two halves, which name no
// constellation, so NewConstellation refuses them as it does any unknown
// name.
func TestNewConstellationRefusesSerpenssHalves(t *testing.T) {
	for _, key := range []string{"SER1", "ser2"} {
		if _, err := constellation.Centroid(key); err != nil {
			t.Fatalf("Centroid(%q): %v, the premise of this test", key, err)
		}

		if _, err := NewConstellation(key); !errors.Is(err, constellation.ErrUnknownAbbreviation) {
			t.Errorf("NewConstellation(%q) error = %v, want ErrUnknownAbbreviation", key, err)
		}
	}
}

// TestNewConstellation_GetDetailsAndWindows confirms Constellation
// composes with the rest of the plan package's Observable-consuming
// machinery (GetDetails, ObservableWindows) with zero special-casing,
// since it implements only the plain Observable interface.
func TestNewConstellation_GetDetailsAndWindows(t *testing.T) {
	c, err := NewConstellation("Ursa Major")
	if err != nil {
		t.Fatalf("NewConstellation: %v", err)
	}

	d, err := c.GetDetails(testContext(t), DetailOverrides{})
	if err != nil {
		t.Fatalf("GetDetails: %v", err)
	}

	if d.Name != "Ursa Major" {
		t.Errorf("GetDetails Name = %q, want %q", d.Name, "Ursa Major")
	}

	loc, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	site, err := NewSite("Test", loc)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	start := time.FromJD(2451545.0, time.UTC)
	end := start.Add(unit.Hours(24))

	if _, err := ObservableWindows(c, start, end, unit.Minutes(10), site); err != nil {
		t.Fatalf("ObservableWindows: %v", err)
	}
}
