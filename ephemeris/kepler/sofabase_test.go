package kepler_test

import (
	"sync"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// TestSOFABaseLocatesAboutItsSeriesCenters: the default base answers each
// body about the center its series computes it from — the Earth about the
// Sun and the Sun about the barycenter from one Epv00, a planet about the
// Sun from Plan94, the Moon about the Earth from Moon98 — each to the bit of
// the series itself, so composing them is the arithmetic State used to do.
//
// The base keeps its last Epv00 for the Sun and the Earth at one epoch. Many
// readers at once, each at its own epoch, must still get their own epoch's
// series, which is what -race and the bit comparison hold it to.
func TestSOFABaseLocatesAboutItsSeriesCenters(t *testing.T) {
	t.Parallel()

	p := kepler.New()
	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	var wg sync.WaitGroup

	for g := range 8 {
		wg.Go(func() {
			for k := range 40 {
				at := start.Add(unit.Days(float64(g*40+k) * 1.3))
				d1, d2 := at.TDB().JDParts()
				pvh, pvb, _ := gofaext.Epv00(d1, d2)
				mars, _ := gofaext.Plan94(d1, d2, 4)
				moon := gofaext.Moon98(d1, d2)

				for _, c := range []struct {
					id, center core.ID
					want       vector.Vec3
				}{
					{core.Earth, core.Sun, vector.V3(pvh[0][0], pvh[0][1], pvh[0][2])},
					{core.Sun, core.SolarSystemBarycenter, vector.V3(pvb[0][0]-pvh[0][0], pvb[0][1]-pvh[0][1], pvb[0][2]-pvh[0][2])},
					{core.MarsBarycenter, core.Sun, vector.V3(mars[0][0], mars[0][1], mars[0][2])},
					{core.Moon, core.Earth, vector.V3(moon[0][0], moon[0][1], moon[0][2])},
				} {
					st, err := p.Locate(c.id, at)
					if err != nil {
						t.Errorf("Locate(%v): %v", c.id, err)

						return
					}

					if st.Center != c.center || st.Pos != c.want {
						t.Errorf("Locate(%v) at day %d: %v about %v, want %v about %v", c.id, g*40+k, st.Pos, st.Center, c.want, c.center)
					}
				}
			}
		})
	}

	wg.Wait()
}
