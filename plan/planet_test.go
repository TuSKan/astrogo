package plan

import (
	"testing"

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
