package plan

import (
	"errors"
	"math"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// TestPlanetaryMoonsTableIntegrity guards moonSpecs against copy-paste
// mistakes that would silently corrupt gatherPlanetaryMoons' output: a
// duplicate NAIF ID (two moons colliding on the same body), an empty name
// or kernel, or an H value outside any real moon's plausible range (a
// typo — e.g. a transposed digit — would otherwise surface exactly like
// the impossible-magnitude bug this session already found and fixed in
// ephemeris/jpl/spk's Type 21 reader).
func TestPlanetaryMoonsTableIntegrity(t *testing.T) {
	seenID := make(map[int32]string)

	for key, m := range moonSpecs {
		if m.name == "" {
			t.Errorf("moonSpec with NAIF ID %d has an empty name", m.naifID)
		}

		if normalizeSiteName(m.name) != key {
			t.Errorf("moonSpecs key %q does not match normalized name of %q", key, m.name)
		}

		if m.kernel == "" {
			t.Errorf("moonSpec %q has an empty kernel", m.name)
		}

		if prev, ok := seenID[m.naifID]; ok {
			t.Errorf("NAIF ID %d used by both %q and %q", m.naifID, prev, m.name)
		}

		seenID[m.naifID] = m.name

		// The faintest real absolute magnitude among these moons (Deimos,
		// H≈12.89) and the brightest (Ganymede, H≈-2.09) bound a generous
		// plausible range — anything outside it is almost certainly a typo,
		// not a real published value.
		if m.h < -5 || m.h > 20 {
			t.Errorf("%s: H=%.2f is outside a plausible range for a named Solar System moon", m.name, m.h)
		}
	}

	if len(moonSpecs) != 21 {
		t.Errorf("expected 21 planetary moons, got %d", len(moonSpecs))
	}
}

// TestNewPlanetaryMoonUnknownName verifies the ErrUnknownPlanetaryMoon
// sentinel path.
func TestNewPlanetaryMoonUnknownName(t *testing.T) {
	prov := newOppositionProvider(501)

	if _, err := NewPlanetaryMoon("Not A Real Moon", prov); !errors.Is(err, ErrUnknownPlanetaryMoon) {
		t.Errorf("NewPlanetaryMoon(nonexistent) error = %v, want ErrUnknownPlanetaryMoon", err)
	}
}

// TestPlanetaryMoon_ParentAndEmbedding confirms PlanetaryMoon records its
// parent planet correctly, and that embedding *Asteroid gives it identical
// behavior to an equivalent bare *Asteroid (Position/GeocentricVec/
// ApparentMagnitude all promoted unchanged) — an embedding-correctness
// check, not a re-test of Asteroid's own physics (already covered by
// TestAsteroid_HG_OppositionMagnitude etc. in moving_bodies_test.go).
func TestPlanetaryMoon_ParentAndEmbedding(t *testing.T) {
	const moonID eph.ID = 501 // Io's real NAIF ID

	prov := newOppositionProvider(moonID)
	tm := time.FromJD(2451545.0, time.UTC)

	moon, err := NewPlanetaryMoon("Io", prov)
	if err != nil {
		t.Fatalf("NewPlanetaryMoon: %v", err)
	}

	asteroid := NewAsteroid("Io", moonID, prov, WithHG(-1.68, moonDefaultG))

	if got := moon.Parent(); got != eph.Jupiter {
		t.Errorf("Parent() = %v, want %v", got, eph.Jupiter)
	}

	if moon.Name() != "Io" {
		t.Errorf("Name() = %q, want %q", moon.Name(), "Io")
	}

	if moon.EphID() != moonID {
		t.Errorf("EphID() = %v, want %v", moon.EphID(), moonID)
	}

	moonMag, err := moon.ApparentMagnitude(tm)
	if err != nil {
		t.Fatalf("PlanetaryMoon.ApparentMagnitude: %v", err)
	}

	asteroidMag, err := asteroid.ApparentMagnitude(tm)
	if err != nil {
		t.Fatalf("Asteroid.ApparentMagnitude: %v", err)
	}

	if moonMag != asteroidMag {
		t.Errorf("PlanetaryMoon.ApparentMagnitude = %v, want it to match the equivalent Asteroid's %v", moonMag, asteroidMag)
	}

	moonPos, err := moon.Position(tm)
	if err != nil {
		t.Fatalf("PlanetaryMoon.Position: %v", err)
	}

	asteroidPos, err := asteroid.Position(tm)
	if err != nil {
		t.Fatalf("Asteroid.Position: %v", err)
	}

	if moonPos.RA() != asteroidPos.RA() || moonPos.Dec() != asteroidPos.Dec() {
		t.Errorf("PlanetaryMoon.Position = %v, want it to match the equivalent Asteroid's %v", moonPos, asteroidPos)
	}

	moonVec, err := moon.GeocentricVec(tm)
	if err != nil {
		t.Fatalf("PlanetaryMoon.GeocentricVec: %v", err)
	}

	if math.Abs(moonVec.Norm()-1.77) > 1e-9 {
		t.Errorf("PlanetaryMoon.GeocentricVec norm = %v, want 1.77 AU", moonVec.Norm())
	}
}

// TestPlanetaryMoon_CaseInsensitiveLookup confirms NewPlanetaryMoon matches
// name case- and space-insensitively, like NewKnownSite/NewMeteorShower.
func TestPlanetaryMoon_CaseInsensitiveLookup(t *testing.T) {
	prov := newOppositionProvider(606)

	for _, name := range []string{"Titan", "titan", "TITAN"} {
		moon, err := NewPlanetaryMoon(name, prov)
		if err != nil {
			t.Errorf("NewPlanetaryMoon(%q): %v", name, err)
			continue
		}

		if moon.Name() != "Titan" {
			t.Errorf("NewPlanetaryMoon(%q).Name() = %q, want %q", name, moon.Name(), "Titan")
		}

		if moon.Parent() != eph.Saturn {
			t.Errorf("NewPlanetaryMoon(%q).Parent() = %v, want %v", name, moon.Parent(), eph.Saturn)
		}
	}
}

// TestPlanetaryMoon_GetDetails is a light sanity check that GetDetails
// (promoted from *Asteroid) works end-to-end for a *PlanetaryMoon.
func TestPlanetaryMoon_GetDetails(t *testing.T) {
	const moonID eph.ID = 606 // Titan

	prov := newOppositionProvider(moonID)

	moon, err := NewPlanetaryMoon("Titan", prov)
	if err != nil {
		t.Fatalf("NewPlanetaryMoon: %v", err)
	}

	d, err := moon.GetDetails(testContext(t))
	if err != nil {
		t.Fatalf("GetDetails: %v", err)
	}

	if d.Name != "Titan" {
		t.Errorf("GetDetails Name = %q, want %q", d.Name, "Titan")
	}
}

// Interface-satisfaction is also enforced at compile time in
// plan/observable.go's assertion block; this is a belt-and-suspenders
// runtime check via explicit assignment.
func TestPlanetaryMoon_ImplementsInterfaces(_ *testing.T) {
	var (
		_ Observable        = (*PlanetaryMoon)(nil)
		_ MovingBody        = (*PlanetaryMoon)(nil)
		_ MagnitudeComputer = (*PlanetaryMoon)(nil)
	)
}
