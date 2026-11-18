package plan

import (
	"testing"

	"github.com/TuSKan/astrogo/catalog/resolve"
	eph "github.com/TuSKan/astrogo/ephemeris"
)

// TestEveryPlanetIsItsSystemBarycenter holds the planet constructors to the
// one definition Planet states: a planet is its system barycenter, NAIF 1 to
// 9, Mercury through Pluto alike.
func TestEveryPlanetIsItsSystemBarycenter(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		ctor func(eph.Provider) *Planet
		want eph.ID
	}{
		{NewMercury, eph.MercuryBarycenter},
		{NewVenus, eph.VenusBarycenter},
		{NewMars, eph.MarsBarycenter},
		{NewJupiter, eph.JupiterBarycenter},
		{NewSaturn, eph.SaturnBarycenter},
		{NewUranus, eph.UranusBarycenter},
		{NewNeptune, eph.NeptuneBarycenter},
		{NewPluto, eph.PlutoBarycenter},
	} {
		p := c.ctor(nil)
		if p.EphID() != c.want {
			t.Errorf("%s is NAIF %d, want its system barycenter, %d", p.Name(), p.EphID(), c.want)
		}
	}
}

// TestACatalogPlanetIsTheConstructorsPoint: a catalog target naming a planet
// by its own NAIF code or its barycenter's becomes the point the planet's
// constructor tracks, so FromCatalog and NewX never disagree on what a
// planet is.
func TestACatalogPlanetIsTheConstructorsPoint(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name  string
		spkid string
		ctor  func(eph.Provider) *Planet
	}{
		{"Mercury", "199", NewMercury},
		{"Mercury", "1", NewMercury},
		{"Venus", "299", NewVenus},
		{"Venus", "2", NewVenus},
		{"Mars", "499", NewMars},
		{"Jupiter", "5", NewJupiter},
	} {
		obs, err := FromCatalog(resolve.Target{Name: c.name, SPKID: c.spkid}, nil)
		if err != nil {
			t.Fatalf("FromCatalog(%s, %s): %v", c.name, c.spkid, err)
		}

		p, ok := obs.(*Planet)
		if !ok {
			t.Fatalf("FromCatalog(%s, %s) = %T, want *Planet", c.name, c.spkid, obs)
		}

		if want := c.ctor(nil).EphID(); p.EphID() != want {
			t.Errorf("%s by NAIF %s is NAIF %d; its constructor tracks %d", c.name, c.spkid, p.EphID(), want)
		}
	}
}
