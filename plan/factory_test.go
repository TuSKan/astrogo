package plan_test

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/catalog/resolve"
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
