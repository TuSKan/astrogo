package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// itrsContext is the observer these tests rotate around: a real site, a fixed
// epoch, and no atmosphere, since nothing here is refracted.
func itrsContext(t *testing.T) *coord.Context {
	t.Helper()

	site := coord.MustGeodetic(angle.Deg(-70.40417), angle.Deg(-24.62722), 2635) // Paranal

	return coord.NewContext(
		time.Date(2026, 4, 15, 22, 0, 0, 0, time.LocationUTC),
		site,
		atmosphere.Refraction{},
	)
}

// TestICRSToITRSIsSOFAsC2t06a pins the claim [coord.Context] makes in a
// comment and nothing previously checked: that building the rotation from its
// cached factors is *bit-identical* to calling SOFA's one-shot routine.
//
// The Context keeps precession-nutation and polar motion separately so that
// AtTime can rebuild the matrix from a fresh Earth rotation angle alone, which
// is what makes a time step cheap. That is an optimisation of C2t06a, and an
// optimisation of a reference routine is only safe while it still agrees with
// it — so the comparison is exact equality rather than a tolerance. A
// tolerance here would let the factored form drift and call it rounding.
func TestICRSToITRSIsSOFAsC2t06a(t *testing.T) {
	t.Parallel()

	ctx := itrsContext(t)

	// Rebuilt from the Context's own epoch and Earth orientation, through the
	// single-call routine rather than the factored one.
	eop := ctx.Time().EOP()
	tt1, tt2 := ctx.Time().TT().JDParts()
	utc1, utc2 := ctx.Time().UTC().JDParts()

	mat := gofaext.C2t06a(tt1, tt2, utc1, utc2+eop.DUT1/86400.0, eop.XP, eop.YP)

	for _, v := range []vector.Vec3{
		{X: 1, Y: 0, Z: 0},
		{X: 0, Y: 1, Z: 0},
		{X: 0, Y: 0, Z: 1},
		{X: 0.3, Y: -0.6, Z: 0.74},
		{X: -1234.5, Y: 6789.0, Z: -42.0},
	} {
		want := vector.Vec3{
			X: mat[0][0]*v.X + mat[0][1]*v.Y + mat[0][2]*v.Z,
			Y: mat[1][0]*v.X + mat[1][1]*v.Y + mat[1][2]*v.Z,
			Z: mat[2][0]*v.X + mat[2][1]*v.Y + mat[2][2]*v.Z,
		}

		if got := ctx.ICRSToITRS(v); got != want {
			t.Errorf("ICRSToITRS(%v)\n got %v\nwant %v (SOFA's C2t06a)", v, got, want)
		}
	}
}

// TestITRSRoundTripsAndKeepsItsLength checks the two properties a rotation has
// and a general linear map does not.
//
// The inverse is applied as the transpose rather than computed, so a round trip
// that lost precision would mean the matrix had stopped being orthogonal —
// which for this matrix would mean one of its three factors had.
func TestITRSRoundTripsAndKeepsItsLength(t *testing.T) {
	t.Parallel()

	ctx := itrsContext(t)

	for _, v := range []vector.Vec3{
		{X: 1, Y: 0, Z: 0},
		{X: 0.3, Y: -0.6, Z: 0.74},
		{X: 6378137, Y: 0, Z: 0},       // an equatorial Earth radius, in metres
		{X: -1e-9, Y: 2e-9, Z: 3.5e-9}, // and something tiny
		{X: 1.5e11, Y: -2e11, Z: 1e10}, // and something the size of an orbit
	} {
		itrs := ctx.ICRSToITRS(v)

		testutil.AssertNear(t, "length preserved",
			itrs.Norm(), v.Norm(), 1e-12*math.Max(1, v.Norm()))

		back := ctx.ITRSToICRS(itrs)

		for _, c := range []struct {
			name      string
			got, want float64
		}{
			{"x", back.X, v.X},
			{"y", back.Y, v.Y},
			{"z", back.Z, v.Z},
		} {
			// The tolerance scales with the vector's length, not the
			// component's: rotating (6378137, 0, 0) puts a little of that
			// magnitude into every axis, so a zero component comes back as a
			// rounding residual of the whole, not of itself.
			testutil.AssertNear(t, "round trip "+c.name, c.got, c.want,
				1e-12*math.Max(1, v.Norm()))
		}
	}
}

// TestITRSTurnsAtTheSiderealRate is the external check: the rotation's job is
// to carry the Earth's spin, and that rate is a defined constant rather than
// anything astrogo chooses.
//
// A direction fixed in the sky sweeps through Earth-fixed longitude at the
// rate the Earth turns against the stars — 1.00273781191135448 turns per UT1
// day, or 15.0410672 degrees an hour. A solar rate of 15.0000 would mean the
// matrix was being driven by UTC rather than UT1, which is a mistake that
// costs 4 minutes a day and looks almost right for the first hour.
func TestITRSTurnsAtTheSiderealRate(t *testing.T) {
	t.Parallel()

	ctx := itrsContext(t)
	later := ctx.AtTime(ctx.Time().AddDays(1.0 / 24))

	star := coord.NewICRS(angle.Deg(101.2871), angle.Deg(-16.7161)).ToUnitVector()

	lon0, _ := ctx.ICRSToITRS(star).ToSpherical()
	lon1, _ := later.ICRSToITRS(star).ToSpherical()

	// The star falls behind the rotating Earth, so its Earth-fixed longitude
	// decreases; the wrap makes the raw difference useless.
	turned := -angle.Rad(lon1 - lon0).Wrap180().Degrees()

	const siderealDegPerHour = 360 * 1.00273781191135448 / 24

	t.Logf("one hour of UT1 turned the frame by %.7f degrees (sidereal %.7f, solar 15.0)",
		turned, siderealDegPerHour)

	// A tenth of a milliarcsecond. Precession and nutation are held fixed
	// across the step by design, and over an hour they are far below this.
	if diff := math.Abs(turned - siderealDegPerHour); diff > 1e-4/3600 {
		t.Errorf("the frame turned %.9f degrees in an hour, want %.9f (differ by %.4f mas) — "+
			"15.0 would mean the rotation is being driven by UTC instead of UT1",
			turned, siderealDegPerHour, diff*3600*1000)
	}
}

// TestTheObserverRotatesOntoItsOwnSite is the end-to-end one, and it closes a
// loop between three pieces that were built separately.
//
// A Context caches the observer's position as an ICRS vector, computed by
// SOFA's Pvtob from the site's geodetic coordinates and the Earth orientation
// of the moment. Rotating that vector into the Earth-fixed frame and asking
// [coord.FromECEF] where it is has to give the observatory back — polar motion
// included, since Pvtob applies it and the rotation undoes it.
//
// Nothing here shares code: one path is SOFA's, one is this change's, and the
// geodetic inversion is astrogo's own. A sign error in any of the three would
// put the site somewhere else on Earth.
func TestTheObserverRotatesOntoItsOwnSite(t *testing.T) {
	t.Parallel()

	ctx := itrsContext(t)
	site := ctx.Site()

	// ObsVec is in AU; the ellipsoid works in metres.
	au := constants.IAU.AstronomicalUnit.Value

	ecef := ctx.ICRSToITRS(ctx.ObsVec()).MulScalar(au)

	got, err := coord.FromECEF(ecef, coord.WGS84())
	if err != nil {
		t.Fatalf("FromECEF: %v", err)
	}

	t.Logf("site     lon %.9f lat %.9f h %.4f m", site.Lon().Degrees(), site.Lat().Degrees(), site.Height())
	t.Logf("recovered lon %.9f lat %.9f h %.4f m", got.Lon().Degrees(), got.Lat().Degrees(), got.Height())

	// A milliarcsecond of latitude is about 3 cm on the ground, which is the
	// scale polar motion works at and well inside what this can resolve.
	const masDeg = 1.0 / 3600 / 1000

	testutil.AssertNear(t, "longitude (deg)", got.Lon().Degrees(), site.Lon().Degrees(), masDeg)
	testutil.AssertNear(t, "latitude (deg)", got.Lat().Degrees(), site.Lat().Degrees(), masDeg)
	testutil.AssertNear(t, "height (m)", got.Height().Meters(), site.Height().Meters(), 0.01)
}

func BenchmarkICRSToITRS(b *testing.B) {
	ctx := coord.NewContext(
		time.Date(2026, 4, 15, 22, 0, 0, 0, time.LocationUTC),
		coord.MustGeodetic(angle.Deg(-70.40417), angle.Deg(-24.62722), 2635),
		atmosphere.Refraction{},
	)

	v := vector.V3(0.3, -0.6, 0.74)

	b.ReportAllocs()

	for range b.N {
		_ = ctx.ICRSToITRS(v)
	}
}
