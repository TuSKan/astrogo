package plan

import (
	"testing"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// customBody is a MovingBody a caller might write, as #791's report did with
// a first-order place: it embeds a *Planet but answers GeocentricVec itself,
// so this package cannot assume its place is the ephemeris's, even though
// embedding gives it every method a *Planet has.
type customBody struct {
	*Planet
}

func (c customBody) GeocentricVec(t epoch.Time) (vector.Vec3, error) {
	return c.Planet.GeocentricVec(t)
}

// TestObservedPlaceRoutesEachKindOfBody: this package's ephemeris targets are
// observed from the site itself (#840), bit for bit what the ephemeris and
// coord give that way; a MovingBody implemented elsewhere keeps its own
// GeocentricVec and the geocentric route, bit for bit too. For the Moon the
// two routes differ, which is what makes the first check mean something.
func TestObservedPlaceRoutesEachKindOfBody(t *testing.T) {
	t.Parallel()

	p := eph.Default()

	site, err := coord.NewGeodetic(angle.LatDeg(41.3874), angle.LonDeg(2.1686), unit.Meters(0))
	if err != nil {
		t.Fatal(err)
	}

	at := epoch.Date(2026, time.June, 10, 21, 0, 0, 0, time.UTC)

	ctx, err := coord.NewContext(at, site, atmosphere.Refraction{})
	if err != nil {
		t.Fatal(err)
	}

	moon := NewMoon(p)

	got, err := observedPlace(moon, at, ctx)
	if err != nil {
		t.Fatal(err)
	}

	app, err := eph.At(p, at).Apparent(eph.Moon, eph.Site(ctx))
	if err != nil {
		t.Fatal(err)
	}

	if want := ctx.TopocentricToObserved(app.Pos); got != want {
		t.Errorf("the Moon's observed place %v, the Site route's %v", got, want)
	}

	custom, err := observedPlace(customBody{moon}, at, ctx)
	if err != nil {
		t.Fatal(err)
	}

	geo, err := moon.GeocentricVec(at)
	if err != nil {
		t.Fatal(err)
	}

	if want := ctx.GeocentricToObserved(geo); custom != want {
		t.Errorf("a caller's MovingBody observed at %v, its geocentric route at %v", custom, want)
	}

	if custom == got {
		t.Error("the two routes agree for the Moon, so this test does not tell them apart")
	}
}
