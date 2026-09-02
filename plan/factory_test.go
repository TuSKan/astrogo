package plan_test

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

// errStubMoonProviderState is returned by stubMoonProvider's State — this
// test never exercises State (construction alone is enough to check the
// routing), so the value only needs to exist, not be reachable.
var errStubMoonProviderState = errors.New("stubMoonProvider: not implemented")

// stubMoonProvider is a minimal eph.Provider whose State always errors —
// FromCatalog's PlanetaryMoon routing only needs construction to succeed
// (plan.NewPlanetaryMoon/NewAsteroid never call the provider eagerly), so
// no real ephemeris data is needed to exercise the routing itself.
type stubMoonProvider struct{}

func (stubMoonProvider) State(eph.ID, time.Time) (eph.State, error) {
	return eph.State{}, errStubMoonProviderState
}

func (stubMoonProvider) Close() error { return nil }

// TestFromCatalog_PlanetaryMoon is a regression test for FromCatalog
// silently degrading a resolve.KindPlanetaryMoon candidate to a
// *plan.GenericBody with no photometric model — plan/moons.go's
// gatherPlanetaryMoons already tags candidates this way, but FromCatalog
// had no case for it, so any caller round-tripping a moon target through
// the catalog layer (rather than calling plan.NewPlanetaryMoon directly)
// lost the H-G reflectance model. A known name must now resolve to a real
// *plan.PlanetaryMoon with the right parent planet.
func TestFromCatalog_PlanetaryMoon(t *testing.T) {
	c := catalog.Target{
		Name: "Io",
		Kind: resolve.KindPlanetaryMoon,
	}

	obj := plan.FromCatalog(c, stubMoonProvider{})

	moon, ok := obj.(*plan.PlanetaryMoon)
	if !ok {
		t.Fatalf("FromCatalog: got %T, want *plan.PlanetaryMoon", obj)
	}

	if moon.Parent() != eph.Jupiter {
		t.Errorf("Parent() = %v, want eph.Jupiter", moon.Parent())
	}
}

// TestFromCatalog_UnknownPlanetaryMoonFallsThrough confirms an unrecognized
// name (not in plan's fixed moon table) still falls through to the generic
// path rather than being dropped entirely — FromCatalog has no error
// return, so a lookup failure must degrade gracefully, not panic or nil-out.
func TestFromCatalog_UnknownPlanetaryMoonFallsThrough(t *testing.T) {
	c := catalog.Target{
		Name: "NotARealMoon",
		Kind: resolve.KindPlanetaryMoon,
	}

	obj := plan.FromCatalog(c, stubMoonProvider{})

	if _, ok := obj.(*plan.GenericBody); !ok {
		t.Fatalf("FromCatalog: got %T, want *plan.GenericBody", obj)
	}
}

// ceresLikeTarget returns a catalog.Target carrying real, valid (1
// Ceres-like) osculating orbital elements — the Phase 3 "Kepler as the
// default for small bodies" wiring's fixture, shared by the tests below.
func ceresLikeTarget(kind resolve.Kind) catalog.Target {
	return catalog.Target{
		Name:          "1 Ceres",
		ID:            "20000001",
		SPKID:         "20000001",
		Kind:          kind,
		HasElements:   true,
		Epoch:         time.Date(2026, time.June, 9, 0, 0, 0, 0, time.LocationUTC),
		SemiMajorAxis: 2.77,
		Eccentricity:  0.0797,
		Inclination:   angle.Deg(10.6),
		AscendingNode: angle.Deg(80.2),
		ArgPeriapsis:  angle.Deg(73.3),
		MeanAnomaly:   angle.Deg(274),
	}
}

// TestFromCatalog_ElementsToAsteroid confirms FromCatalog builds a
// Kepler-propagated *plan.Asteroid from a target's published elements
// when no provider is supplied — the core new capability of Phase 3:
// "Kepler as the default" for a small body with HasElements=true,
// reached via the exact call (FromCatalog(target, nil)) any ordinary
// caller already makes.
func TestFromCatalog_ElementsToAsteroid(t *testing.T) {
	c := ceresLikeTarget(resolve.KindAsteroid)
	c.H, c.HasH, c.G = 3.34, true, 0.12

	obj := plan.FromCatalog(c, nil)

	ast, ok := obj.(*plan.Asteroid)
	if !ok {
		t.Fatalf("FromCatalog: got %T, want *plan.Asteroid", obj)
	}

	// EphID should be the real SPK ID parsed from c.SPKID (2000001),
	// not the keplerSyntheticID fallback — confirms the real ID, not a
	// sentinel, threads through when one is available.
	if got := ast.EphID(); got != eph.ID(20000001) {
		t.Errorf("EphID() = %v, want 20000001 (parsed from SPKID, not the synthetic fallback)", got)
	}

	pos, err := ast.Position(c.Epoch)
	if err != nil {
		t.Fatalf("Position: %v", err)
	}

	if pos.RA().Radians() == 0 && pos.Dec().Radians() == 0 {
		t.Error("Position returned the zero value — Kepler propagation likely did not run")
	}
}

// TestFromCatalog_ElementsToComet mirrors TestFromCatalog_ElementsToAsteroid
// for the comet (M1/K1) branch.
func TestFromCatalog_ElementsToComet(t *testing.T) {
	c := ceresLikeTarget(resolve.KindComet)
	c.M1, c.HasM1, c.K1 = 4.5, true, 8.0

	obj := plan.FromCatalog(c, nil)

	if _, ok := obj.(*plan.Comet); !ok {
		t.Fatalf("FromCatalog: got %T, want *plan.Comet", obj)
	}
}

// TestFromCatalog_ElementsHyperbolicFallsThrough confirms a target whose
// published eccentricity is >= 1 (which ephemeris/kepler's two-body
// propagator cannot represent — eph.NewElements rejects it with
// ErrUnsupportedOrbit) falls straight through to the fixed-target path
// rather than panicking or silently building a broken Observable, since
// FromCatalog has no error return.
func TestFromCatalog_ElementsHyperbolicFallsThrough(t *testing.T) {
	c := ceresLikeTarget(resolve.KindInterstellar)
	c.Eccentricity = 1.2 // hyperbolic
	c.H, c.HasH = 22.0, true

	obj := plan.FromCatalog(c, nil)

	if _, ok := obj.(*plan.DeepSkyObject); !ok {
		t.Fatalf("FromCatalog: got %T, want the fixed-target fallback *plan.DeepSkyObject", obj)
	}
}

// TestFromCatalog_ProviderTakesPrecedenceOverElements confirms a
// caller-supplied provider always wins over HasElements — the Kepler
// branch is only ever reached when p == nil.
func TestFromCatalog_ProviderTakesPrecedenceOverElements(t *testing.T) {
	c := ceresLikeTarget(resolve.KindAsteroid)
	c.H, c.HasH, c.G = 3.34, true, 0.12

	obj := plan.FromCatalog(c, stubMoonProvider{})

	ast, ok := obj.(*plan.Asteroid)
	if !ok {
		t.Fatalf("FromCatalog: got %T, want *plan.Asteroid", obj)
	}

	if ast.Provider() != (eph.Provider)(stubMoonProvider{}) {
		t.Error("expected the caller-supplied provider to be used, not a Kepler-built one")
	}
}

// TestFromCatalog_StarRadialVelocity confirms a Star target's
// HasRadialVelocity flag is threaded through to the built *plan.Star via
// WithRadialVelocity, including the true-zero case — the merge-rule bug
// this Has-flag exists to prevent (catalog.go silently dropping a
// genuinely-measured 0 km/s value) has no equivalent here since
// FromCatalog reads the flag directly, but the wiring itself still needs
// its own test: nothing previously called FromCatalog with
// HasRadialVelocity set at all.
func TestFromCatalog_StarRadialVelocity(t *testing.T) {
	cases := []struct {
		name    string
		hasRV   bool
		rv      float64
		wantHas bool
	}{
		{"measured negative", true, -5.5, true},
		{"measured true zero", true, 0, true},
		{"unset", false, 0, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tgt := catalog.Target{
				Name:              "Test Star",
				Kind:              resolve.KindStar,
				HasCoord:          true,
				Coord:             coord.NewICRS(angle.Deg(10), angle.Deg(20)),
				HasRadialVelocity: c.hasRV,
				RadialVelocity:    c.rv,
			}

			obj := plan.FromCatalog(tgt, nil)

			star, ok := obj.(*plan.Star)
			if !ok {
				t.Fatalf("FromCatalog: got %T, want *plan.Star", obj)
			}

			gotRV, gotHas := star.MeasuredRadialVelocity()
			if gotHas != c.wantHas {
				t.Errorf("MeasuredRadialVelocity() has = %v, want %v", gotHas, c.wantHas)
			}

			if gotHas && gotRV != c.rv {
				t.Errorf("MeasuredRadialVelocity() rv = %v, want %v", gotRV, c.rv)
			}
		})
	}
}

// TestFromCatalog_AsteroidDiameterAndAlbedo confirms asteroidOptsFrom
// wires SBDB's decoded phys_par Diameter/Albedo through to the built
// *plan.Asteroid's PhysicalRadius() — the measured-diameter and
// albedo-estimate branches are otherwise never exercised by any existing
// FromCatalog test (ceresLikeTarget sets neither).
func TestFromCatalog_AsteroidDiameterAndAlbedo(t *testing.T) {
	t.Run("measured diameter", func(t *testing.T) {
		c := ceresLikeTarget(resolve.KindAsteroid)
		c.H, c.HasH, c.G = 3.34, true, 0.12
		c.HasDiameter, c.Diameter = true, 16.84 // km, real Eros value

		obj := plan.FromCatalog(c, nil)

		ast, ok := obj.(*plan.Asteroid)
		if !ok {
			t.Fatalf("FromCatalog: got %T, want *plan.Asteroid", obj)
		}

		metres, ok := ast.PhysicalRadius()
		if !ok {
			t.Fatal("PhysicalRadius: ok = false, want true (measured diameter present)")
		}

		if want := 16.84 * 1000 / 2; metres != want {
			t.Errorf("PhysicalRadius() = %v, want %v (16.84 km diameter -> radius in metres)", metres, want)
		}
	})

	t.Run("albedo estimate", func(t *testing.T) {
		c := ceresLikeTarget(resolve.KindAsteroid)
		c.H, c.HasH, c.G = 3.34, true, 0.12
		c.HasAlbedo, c.Albedo = true, 0.25 // real Eros value

		obj := plan.FromCatalog(c, nil)

		ast, ok := obj.(*plan.Asteroid)
		if !ok {
			t.Fatalf("FromCatalog: got %T, want *plan.Asteroid", obj)
		}

		metres, ok := ast.PhysicalRadius()
		if !ok {
			t.Fatal("PhysicalRadius: ok = false, want true (albedo present, estimate should apply)")
		}

		if metres <= 0 {
			t.Errorf("PhysicalRadius() = %v, want a positive H+albedo estimate", metres)
		}
	})
}

// TestFromCatalogKeepsADeepSkyRadialVelocity covers the half of the catalog
// that was being discarded.
//
// SIMBAD populates rvz_radvel for any object type — M31 at −300.0 km/s, M87
// at +1256.5 — and for an extragalactic object it is the headline
// measurement, usually quoted as a redshift. FromCatalog read
// HasRadialVelocity in the star branch only, so a galaxy's arrived and was
// dropped on the floor.
func TestFromCatalogKeepsADeepSkyRadialVelocity(t *testing.T) {
	t.Parallel()

	obs := plan.FromCatalog(catalog.Target{
		Name:              "M31-like",
		Kind:              resolve.KindGalaxy,
		Coord:             coord.NewICRS(angle.Deg(10.6847), angle.Deg(41.2688)),
		HasCoord:          true,
		RadialVelocity:    -300.0,
		HasRadialVelocity: true,
	}, nil)

	mrv, ok := obs.(plan.MeasuredRadialVelocity)
	if !ok {
		t.Fatalf("%T does not implement MeasuredRadialVelocity", obs)
	}

	rv, has := mrv.MeasuredRadialVelocity()
	if !has {
		t.Fatal("the catalog radial velocity did not survive FromCatalog")
	}

	if rv != -300.0 {
		t.Errorf("radial velocity = %v, want -300.0", rv)
	}
}

// TestFromCatalogDistinguishesNoRadialVelocityFromZero keeps the same
// distinction HasRadialVelocity exists for. A galaxy at rest relative to the
// barycentre would read 0 km/s, and that is a measurement; a galaxy with no
// published RV also reads 0, and that is not.
func TestFromCatalogDistinguishesNoRadialVelocityFromZero(t *testing.T) {
	t.Parallel()

	base := catalog.Target{
		Name:     "no RV on file",
		Kind:     resolve.KindGalaxy,
		Coord:    coord.NewICRS(angle.Deg(10), angle.Deg(41)),
		HasCoord: true,
	}

	absent, ok := plan.FromCatalog(base, nil).(plan.MeasuredRadialVelocity)
	if !ok {
		t.Fatal("a deep-sky object does not implement MeasuredRadialVelocity")
	}

	if _, has := absent.MeasuredRadialVelocity(); has {
		t.Error("a target with no published RV reports one")
	}

	measured := base
	measured.RadialVelocity, measured.HasRadialVelocity = 0, true

	got, ok := plan.FromCatalog(measured, nil).(plan.MeasuredRadialVelocity)
	if !ok {
		t.Fatal("a deep-sky object does not implement MeasuredRadialVelocity")
	}

	rv, has := got.MeasuredRadialVelocity()
	if !has || rv != 0 {
		t.Errorf("a measured 0 km/s reports (%v, %v), want (0, true)", rv, has)
	}
}
