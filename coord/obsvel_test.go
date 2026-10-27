package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
)

// TestObsVelIsTheVelocityApcoAberratesWith: the Context's astrometry holds the
// observer's barycentric velocity, built by SOFA's Apco13 from Epv00's Earth
// and iauPvtob's site, and ObsVel is the site's part of it. So the Context's
// BarycentricVelocity less Epv00's barycentric Earth at the same TT is
// ObsVel, by a route that shares nothing with ObsVel's own arithmetic. And its
// size is the diurnal-aberration speed, 0.465 km/s·cos(latitude) at the
// geoid.
func TestObsVelIsTheVelocityApcoAberratesWith(t *testing.T) {
	t.Parallel()

	auPerDayInKmPerS := constants.IAU.AstronomicalUnit.Value / 1000 / constants.Derived.JulianDaySeconds.Value

	for _, s := range []struct {
		name          string
		lat, lon, alt float64
	}{
		{"equator", 0, 0, 0},
		{"Barcelona", 41.3874, 2.1686, 12},
		{"Paranal", -24.6272, -70.4045, 2635},
		{"near the pole", 89.9, 120, 0},
	} {
		site, err := coord.NewEarthLocation(s.lat, s.lon, s.alt)
		if err != nil {
			t.Fatal(err)
		}

		for _, at := range []time.Time{
			time.Date(2026, time.March, 20, 0, 0, 0, 0, time.LocationUTC),
			time.Date(2026, time.June, 21, 15, 17, 0, 0, time.LocationUTC),
			time.Date(2031, time.December, 1, 6, 0, 0, 0, time.LocationUTC),
		} {
			ctx := coord.NewContext(at, site, atmosphere.Refraction{})

			d1, d2 := at.TT().JDParts()

			_, pvb, status := gofaext.Epv00(d1, d2)
			if status < 0 {
				t.Fatalf("Epv00 status %d", status)
			}

			bary := ctx.BarycentricVelocity() // km/s
			earthX, earthY, earthZ := pvb[1][0]*auPerDayInKmPerS, pvb[1][1]*auPerDayInKmPerS, pvb[1][2]*auPerDayInKmPerS

			got := ctx.ObsVel().MulScalar(auPerDayInKmPerS)
			wx, wy, wz := bary.X-earthX, bary.Y-earthY, bary.Z-earthZ

			// Epv00's velocity at about 30 km/s, rounded twice and
			// differenced: a few times 1e-14 relative of it.
			if d := math.Hypot(math.Hypot(got.X-wx, got.Y-wy), got.Z-wz); d > 1e-9 {
				t.Errorf("%s at %v: ObsVel is %.3g km/s from Apco's site velocity", s.name, at, d)
			}

			// The speed is the Earth's rotation rate times the distance
			// from its axis: 2π × 1.00273781191135448 / 86400 rad/s times
			// (N + h)·cos φ, about 0.465 km/s at the equator.
			speed := got.Norm()
			want := 0.465 * math.Cos(s.lat*math.Pi/180)

			if math.Abs(speed-want) > 0.002*math.Max(want, 0.01) && math.Abs(speed-want) > 1e-4 {
				t.Errorf("%s at %v: ObsVel's speed %.6f km/s, want about %.6f", s.name, at, speed, want)
			}
		}
	}
}

// TestObsVelIsPerpendicularToTheAxis: the rotation velocity has no component
// along the Earth's axis, the celestial intermediate pole, and none along the
// site's own position vector projected onto the equator: it is ω × r.
func TestObsVelIsPerpendicularToTheAxis(t *testing.T) {
	t.Parallel()

	site, err := coord.NewEarthLocation(-24.6272, -70.4045, 2635)
	if err != nil {
		t.Fatal(err)
	}

	ctx := coord.NewContext(time.Date(2026, time.June, 21, 3, 0, 0, 0, time.LocationUTC), site, atmosphere.Refraction{})

	v, r := ctx.ObsVel(), ctx.ObsVec()

	// v ⊥ r: (ω × r)·r = 0.
	if c := v.Dot(r) / (v.Norm() * r.Norm()); math.Abs(c) > 1e-12 {
		t.Errorf("ObsVel·ObsVec/|·||·| = %.3g, want 0", c)
	}
}
