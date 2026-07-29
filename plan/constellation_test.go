package plan

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constellation"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
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

// TestNewConstellation_GetDetailsAndWindows confirms Constellation
// composes with the rest of the plan package's Observable-consuming
// machinery (GetDetails, ObservableWindows) with zero special-casing,
// since it implements only the plain Observable interface.
func TestNewConstellation_GetDetailsAndWindows(t *testing.T) {
	c, err := NewConstellation("Ursa Major")
	if err != nil {
		t.Fatalf("NewConstellation: %v", err)
	}

	d, err := c.GetDetails(testContext(t))
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
	end := start.Add(24 * time.Hour)

	if _, err := ObservableWindows(c, start, end, 10*time.Minute, site); err != nil {
		t.Fatalf("ObservableWindows: %v", err)
	}
}
