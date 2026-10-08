package plan

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// jplTarget is a Target as catalog/jpl builds one from Horizons'
// "Target body name: <name> (<NAIF ID>)" line: no Kind, no coordinates, the
// NAIF ID as both ID and SPKID.
func jplTarget(name, naif string) resolve.Target {
	return resolve.Target{Catalog: "jpl", Name: name, ID: naif, SPKID: naif}
}

// TestFromCatalogTranslatesJPLMajorBodies: catalog/jpl reports NAIF IDs,
// which astrogo numbers differently. Read as an eph.ID, the JPL Sun (NAIF 10)
// was astrogo's body 10, the Moon, and the Moon (301) and Mars (499) were no
// body at all (#636).
func TestFromCatalogTranslatesJPLMajorBodies(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.LocationUTC)

	for _, c := range []struct {
		name, naif string
		want       eph.ID
	}{
		{"Sun", "10", eph.Sun},
		{"Moon", "301", eph.Moon},
		{"Mars", "499", eph.Mars},
		{"Jupiter Barycenter", "5", eph.Jupiter},
		{"Pluto", "999", eph.Pluto},
	} {
		for _, p := range []eph.Provider{nil, eph.Default()} {
			obj, err := FromCatalog(jplTarget(c.name, c.naif), p)
			if err != nil {
				t.Fatalf("%s (NAIF %s): %v", c.name, c.naif, err)
			}

			planet, ok := obj.(*Planet)
			if !ok {
				t.Fatalf("%s (NAIF %s): got %T, want *Planet", c.name, c.naif, obj)
			}

			got, err := planet.Position(at)
			if err != nil {
				t.Fatalf("%s: Position: %v", c.name, err)
			}

			want, err := NewPlanet(c.name, c.want, eph.Default()).Position(at)
			if err != nil {
				t.Fatalf("%s: reference Position: %v", c.name, err)
			}

			if got != want {
				t.Errorf("%s (NAIF %s): at %v, want astrogo body %d at %v", c.name, c.naif, got, c.want, want)
			}
		}
	}
}

// TestFromCatalogRefusesANAIFBodyItCannotPlace: the Earth-Moon barycenter
// (NAIF 3) is no astrogo body. Read as an eph.ID it was astrogo's Earth,
// seen from the Earth.
func TestFromCatalogRefusesANAIFBodyItCannotPlace(t *testing.T) {
	t.Parallel()

	obj, err := FromCatalog(jplTarget("Earth-Moon Barycenter", "3"), eph.Default())
	if !errors.Is(err, ErrNoCoordinates) {
		t.Errorf("FromCatalog(Earth-Moon Barycenter) = %T, %v, want ErrNoCoordinates", obj, err)
	}
}

// TestNAIFMajorRange: small bodies and spacecraft are not read as NAIF
// major bodies.
func TestNAIFMajorRange(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		spkID string
		ok    bool
	}{
		{"10", true}, {"301", true}, {"999", true},
		{"0", false}, {"1000", false}, {"20000001", false}, {"-125544", false}, {"", false}, {"1982 HG1", false},
	} {
		if _, ok := naifMajorRangeID(c.spkID); ok != c.ok {
			t.Errorf("naifMajorRangeID(%q) = %v, want %v", c.spkID, ok, c.ok)
		}
	}
}
