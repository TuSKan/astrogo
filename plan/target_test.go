package plan

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

func TestStarPosition(t *testing.T) {
	s := NewStar("Sirius", angle.Hour(6.7525), angle.Deg(-16.7161))

	pos, err := s.Position(time.FromJD(2451545.0, time.UTC))
	if err != nil {
		t.Fatalf("Position() error: %v", err)
	}

	if pos.RA().Degrees() == 0 && pos.Dec().Degrees() == 0 {
		t.Error("Position returned zero coordinates for named star")
	}
}

func TestStarNoCoordinate(t *testing.T) {
	s := NewStar("Empty", angle.Zero(), angle.Zero())

	pos, err := s.Position(time.FromJD(2451545.0, time.UTC))
	if err != nil {
		t.Fatalf("Position() error: %v", err)
	}

	if pos.RA().Degrees() != 0 || pos.Dec().Degrees() != 0 {
		t.Error("expected zero coords")
	}
}

func TestPlanetPosition(t *testing.T) {
	p := eph.Default()
	sun := NewSun(p)
	tm := time.FromJD(2451545.0, time.UTC)

	pos, err := sun.Position(tm)
	if err != nil {
		t.Fatalf("Position() error: %v", err)
	}

	if pos.RA().Degrees() == 0 && pos.Dec().Degrees() == 0 {
		t.Error("Sun position returned zero coordinates")
	}
}

func TestMoonPosition(t *testing.T) {
	p := eph.Default()
	moon := NewMoon(p)
	tm := time.FromJD(2451545.0, time.UTC)

	pos, err := moon.Position(tm)
	if err != nil {
		t.Fatalf("Position() error: %v", err)
	}

	raDeg := pos.RA().Degrees()
	if raDeg < 0 || raDeg > 360 {
		t.Errorf("Moon RA out of range: %f", raDeg)
	}
}

func TestPlanetGeocentricVec(t *testing.T) {
	mars := NewMars(eph.Default())
	tm := time.FromJD(2451545.0, time.UTC)

	vec, err := mars.GeocentricVec(tm)
	if err != nil {
		t.Fatalf("GeocentricVec() error: %v", err)
	}

	if vec.X == 0 && vec.Y == 0 && vec.Z == 0 {
		t.Error("Mars geocentric vector is zero")
	}
}

func TestPlanetIdentity(t *testing.T) {
	sun := NewSun(eph.Default())
	moon := NewMoon(eph.Default())

	if !sun.IsSun() {
		t.Error("Sun.IsSun() should be true")
	}

	if sun.IsMoon() {
		t.Error("Sun.IsMoon() should be false")
	}

	if !moon.IsMoon() {
		t.Error("Moon.IsMoon() should be true")
	}
}

func TestDeepSkyObject(t *testing.T) {
	m31 := NewDeepSkyObject("M31", angle.Deg(10.68), angle.Deg(41.27),
		WithDSOMagnitude(3.4),
		WithDSOKind("Galaxy"),
		WithDSOAliases("NGC 224", "M31"),
	)

	if m31.Name() != "M31" {
		t.Errorf("Name() = %q, want M31", m31.Name())
	}

	pos, err := m31.Position(time.FromJD(2451545.0, time.UTC))
	if err != nil {
		t.Fatalf("Position() error: %v", err)
	}

	if pos.RA().Degrees() == 0 {
		t.Error("M31 RA should be non-zero")
	}
}

func TestFromCatalog(t *testing.T) {
	// Star
	star := FromCatalog(catalog.Target{
		Name: "Sirius", Kind: resolve.KindStar, HasCoord: true,
		Coord: coord.NewICRS(angle.Deg(10), angle.Deg(20)),
	}, nil)
	if _, ok := star.(*Star); !ok {
		t.Errorf("FromCatalog Star returned %T, want *Star", star)
	}

	// Planet
	planet := FromCatalog(catalog.Target{
		ID: "11", Name: "Sun", Kind: resolve.KindStar,
	}, eph.Default())
	if p, ok := planet.(*Planet); !ok {
		t.Errorf("FromCatalog Planet returned %T, want *Planet", planet)
	} else if !p.IsSun() {
		t.Error("Expected Sun planet")
	}

	// Planet with no provider supplied — regression test: this used to
	// silently fall through to a static *DeepSkyObject (built from
	// whatever, or no, fixed coordinate happened to be on the target)
	// instead of a real, moving *Planet, since the planet-routing switch
	// only ever ran inside FromCatalog's `if p != nil` block. FromCatalog
	// must fall back to eph.Default() for a major named body instead.
	planetNoProvider := FromCatalog(catalog.Target{
		ID: "4", Name: "Mars", Kind: resolve.KindPlanet,
	}, nil)
	if p, ok := planetNoProvider.(*Planet); !ok {
		t.Errorf("FromCatalog Planet (no provider) returned %T, want *Planet", planetNoProvider)
	} else if p.Name() != "Mars" {
		t.Errorf("FromCatalog Planet (no provider) name = %q, want Mars", p.Name())
	}

	// DSO
	dso := FromCatalog(catalog.Target{
		Name: "M31", Kind: resolve.KindGalaxy, HasCoord: true,
		Coord: coord.NewICRS(angle.Deg(10), angle.Deg(41)),
	}, nil)
	if _, ok := dso.(*DeepSkyObject); !ok {
		t.Errorf("FromCatalog DSO returned %T, want *DeepSkyObject", dso)
	}
}

// TestNewPlanet_NilProviderDefaults is a regression test at the
// constructor level, not just FromCatalog's — a direct call like
// plan.NewMars(nil) (bypassing FromCatalog entirely) used to construct
// successfully but fail at the first Position()/ApparentMagnitude() call,
// since NewPlanet stored whatever provider it was given, nil included.
// Every convenience constructor (NewSun, NewMoon, NewMercury...NewPluto)
// delegates through NewPlanet, so fixing it there covers all of them.
func TestNewPlanet_NilProviderDefaults(t *testing.T) {
	epoch := time.FromJD(2451545.0, time.UTC)

	mars := NewMars(nil)
	if _, err := mars.Position(epoch); err != nil {
		t.Errorf("NewMars(nil).Position(): unexpected error: %v", err)
	}

	sun := NewSun(nil)
	if _, err := sun.Position(epoch); err != nil {
		t.Errorf("NewSun(nil).Position(): unexpected error: %v", err)
	}

	moon := NewMoon(nil)
	if _, err := moon.Position(epoch); err != nil {
		t.Errorf("NewMoon(nil).Position(): unexpected error: %v", err)
	}

	pluto := NewPluto(nil)
	if _, err := pluto.Position(epoch); err != nil {
		t.Errorf("NewPluto(nil).Position(): unexpected error: %v", err)
	}
}
