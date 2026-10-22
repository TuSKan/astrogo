package ephemeris_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// TestSiteApparentIsSOFAsTopocentricPlace holds the place seen from a Site,
// carried to the horizon by TopocentricToObserved, to SOFA's own route for a
// star: iauAtciq and iauAtioq (coord.Context.AstrometricToObserved) applied to
// the astrometric place seen from the site. Barcelona, hourly through June
// 2026, the Moon and Mars above 5° and more than 20° from the Sun, no
// refraction.
//
// The two differ by the light deflection alone: Atciq deflects as for a star
// at infinity, 4.07 mas·cot(E/2) at elongation E for an observer at 1 au,
// while the Site place deflects for the body where it is, which for the Moon
// is under 0.01 mas. So the bound is that star deflection plus 0.1 mas.
//
// The geocentric route, GeocentricToObserved on the geocentric apparent place,
// breaks the bound for the Moon by an order of magnitude: it carries annual
// aberration along the geocentric line of sight, which the parallax then turns
// by up to 0.95° (#840). That is asserted too, so the bound can fail.
func TestSiteApparentIsSOFAsTopocentricPlace(t *testing.T) {
	t.Parallel()

	p := eph.Default()

	site, err := coord.NewEarthLocation(41.3874, 2.1686, 0)
	if err != nil {
		t.Fatal(err)
	}

	start := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.LocationUTC)

	for _, body := range []eph.ID{eph.Moon, eph.MarsBarycenter} {
		var (
			n                 int
			worst, worstOld   float64 // excess over the bound, mas
			worstSite, oldMax float64 // raw differences from Atciq, mas
		)

		for h := range 30 * 24 {
			at := start.Add(unit.Hours(float64(h)))
			ctx := coord.NewContext(at, site, atmosphere.Refraction{})
			in := eph.At(p, at)

			app, err := in.Apparent(body, eph.Site(ctx))
			if err != nil {
				t.Fatal(err)
			}

			got := ctx.TopocentricToObserved(app.Pos)
			if got.Alt().Degrees() < 5 {
				continue
			}

			sun, err := in.Geometric(eph.Sun, eph.Earth)
			if err != nil {
				t.Fatal(err)
			}

			e := angleBetween(sun.Pos, app.Pos)
			if e < 20*math.Pi/180 {
				continue
			}

			ast, err := in.Astrometric(body, eph.Site(ctx))
			if err != nil {
				t.Fatal(err)
			}

			ra, dec := ast.Pos.ToSpherical()
			want := ctx.AstrometricToObserved(coord.NewAstrometric(angle.Rad(ra), angle.Rad(dec)))

			geo, err := in.Apparent(body, eph.Geocenter())
			if err != nil {
				t.Fatal(err)
			}

			old := ctx.GeocentricToObserved(geo.Pos)

			// The star deflection Atciq applies, for the Earth's distance from
			// the Sun at this instant, plus room for everything else.
			bound := 4.07/sun.Pos.Norm()/math.Tan(e/2) + 0.1

			d, dOld := observedMas(got, want), observedMas(old, want)
			worst, worstOld = math.Max(worst, d-bound), math.Max(worstOld, dOld-bound)
			worstSite, oldMax = math.Max(worstSite, d), math.Max(oldMax, dOld)
			n++
		}

		if n < 100 {
			t.Fatalf("%v: only %d epochs above 5° and 20° from the Sun", body, n)
		}

		t.Logf("%v: %d epochs; from Atciq, Site %.3f mas, geocentric route %.3f mas at worst", body, n, worstSite, oldMax)

		if worst > 0 {
			t.Errorf("%v: the Site place is %.3f mas beyond the deflection bound from SOFA's Atciq", body, worst)
		}

		if body == eph.Moon && worstOld < 100 {
			t.Errorf("Moon: the geocentric route exceeds the bound by only %.3f mas; the test no longer tells the routes apart", worstOld)
		}
	}
}

// TestSiteIsTheContextsObserver: a Site observer is the Context's own site,
// so the astrometric place seen from it is the geocentric one less the site's
// position, plus the Moon's motion over the difference of the two light
// times. The astrometric place is barycentric, the target at its retarded
// epoch less the observer now, so that motion is the Moon's barycentric
// velocity, about 30 km/s, over at most 21 ms: up to 0.6 km. What is left is
// each light-time solve's epoch as SOFA represents it, 0.16 µs, which is
// 4.8 mm of the Moon's barycentric motion (docs/ephemeris.md): measured 3.5 mm,
// held to 1 cm. The Moon's acceleration over 21 ms is micrometers.
func TestSiteIsTheContextsObserver(t *testing.T) {
	t.Parallel()

	p := eph.Default()

	site, err := coord.NewEarthLocation(-24.6272, -70.4045, 2635)
	if err != nil {
		t.Fatal(err)
	}

	at := time.Date(2026, time.June, 10, 3, 0, 0, 0, time.LocationUTC)
	ctx := coord.NewContext(at, site, atmosphere.Refraction{})
	in := eph.At(p, at)

	fromSite, err := in.Astrometric(eph.Moon, eph.Site(ctx))
	if err != nil {
		t.Fatal(err)
	}

	geo, err := in.Astrometric(eph.Moon, eph.Geocenter())
	if err != nil {
		t.Fatal(err)
	}

	earth, err := in.Geometric(eph.Earth, eph.SolarSystemBarycenter)
	if err != nil {
		t.Fatal(err)
	}

	// τ_geocenter − τ_site, the light times being the ranges over c; and the
	// Moon's barycentric velocity, its geocentric astrometric velocity being
	// relative to the Earth.
	dTau := (geo.Pos.Norm() - fromSite.Pos.Norm()) / (299792458.0 * 86400 / 149597870700)
	moonBary := geo.Vel.Add(earth.Vel)

	want := geo.Pos.Sub(ctx.ObsVec()).Add(moonBary.MulScalar(dTau))

	if d := fromSite.Pos.Sub(want).Norm() * 149597870.7; d > 1e-5 {
		t.Errorf("the Moon seen from the Site is %.3g km from the geocentric place less the site", d)
	}
}

// angleBetween is the angle between two vectors, in radians.
func angleBetween(a, b vector.Vec3) float64 {
	return math.Atan2(a.Cross(b).Norm(), a.Dot(b))
}

// observedMas is the angle between two observed places, in milliarcseconds.
func observedMas(a, b coord.AltAz) float64 {
	u := vector.FromSpherical(a.Az().Radians(), a.Alt().Radians())
	w := vector.FromSpherical(b.Az().Radians(), b.Alt().Radians())

	return angleBetween(u, w) * 180 / math.Pi * 3600e3
}
