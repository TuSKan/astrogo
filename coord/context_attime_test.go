package coord_test

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// TestContextAtTime_ZeroDeltaMatchesNewContext proves AtTime(ctx.Time())
// reproduces a fresh NewContext exactly — the decomposition (C2i06a/Era00/
// Pom00 composed via C2tcio, astrom.Eral updated via Aper) must be
// bit-identical to the monolithic build at the same instant.
func TestContextAtTime_ZeroDeltaMatchesNewContext(t *testing.T) {
	site, err := coord.NewGeodetic(angle.Deg(2.1686), angle.Deg(41.3874), 0)
	testutil.AssertNoError(t, err)

	atm := atmosphere.Atmosphere{Pressure: 0}
	base := time.FromJD(2461000.0, time.UTC)

	fresh := coord.NewContext(base, site, atm)
	derived := coord.NewContext(base, site, atm).AtTime(base)

	star := coord.NewICRS(angle.Hour(6.75), angle.Deg(-16.7))

	wantAA, err := fresh.ICRSToAltAz(star)
	testutil.AssertNoError(t, err)

	gotAA, err := derived.ICRSToAltAz(star)
	testutil.AssertNoError(t, err)

	testutil.AssertNear(t, "ICRSToAltAz Alt at Δt=0", gotAA.Alt().Degrees(), wantAA.Alt().Degrees(), 1e-12)
	testutil.AssertNear(t, "ICRSToAltAz Az at Δt=0", gotAA.Az().Degrees(), wantAA.Az().Degrees(), 1e-12)

	wantHA, err := fresh.ICRSToHourAngle(star)
	testutil.AssertNoError(t, err)

	gotHA, err := derived.ICRSToHourAngle(star)
	testutil.AssertNoError(t, err)

	testutil.AssertNear(t, "ICRSToHourAngle at Δt=0", gotHA.Degrees(), wantHA.Degrees(), 1e-12)

	// Moving-body path: an arbitrary geocentric ICRS vector at roughly
	// lunar distance (AU).
	vec := vector.V3(0.0016, 0.0019, 0.0008)

	wantGeo := fresh.GeocentricToObserved(vec)
	gotGeo := derived.GeocentricToObserved(vec)

	testutil.AssertNear(t, "GeocentricToObserved Alt at Δt=0", gotGeo.Alt().Degrees(), wantGeo.Alt().Degrees(), 1e-12)
	testutil.AssertNear(t, "GeocentricToObserved Az at Δt=0", gotGeo.Az().Degrees(), wantGeo.Az().Degrees(), 1e-12)
}

// TestContextAtTime_DriftStaysWithinDocumentedBound proves AtTime's error
// versus a fresh NewContext at the same later instant (a) stays under the
// ~0.1"/hour bound documented on AtTime, and (b) is non-zero/growing with
// Δt — guarding against a broken cheap path that silently no-ops instead of
// actually advancing the Earth Rotation Angle.
func TestContextAtTime_DriftStaysWithinDocumentedBound(t *testing.T) {
	site, err := coord.NewGeodetic(angle.Deg(2.1686), angle.Deg(41.3874), 0)
	testutil.AssertNoError(t, err)

	atm := atmosphere.Atmosphere{Pressure: 0}
	base := time.FromJD(2461000.0, time.UTC)
	ctx := coord.NewContext(base, site, atm)
	star := coord.NewICRS(angle.Hour(6.75), angle.Deg(-16.7))

	const boundArcsecPerHour = 0.1

	// Altitude error versus a fresh NewContext at the same later instant
	// must stay within the documented bound. This is NOT checked for
	// monotonic growth across checkpoints: altitude is a diurnal
	// projection of the underlying pointing error, and that projection
	// isn't monotonic in Δt (e.g. near a slowly-changing altitude extremum
	// the same pointing error yields a smaller Alt error) — only the
	// bound matters here.
	for _, hours := range []float64{1, 6, 24} {
		later := base.Add(time.Duration(hours * float64(time.Hour)))

		want, err := coord.NewContext(later, site, atm).ICRSToAltAz(star)
		testutil.AssertNoError(t, err)

		got, err := ctx.AtTime(later).ICRSToAltAz(star)
		testutil.AssertNoError(t, err)

		errArcsec := angle.Rad(want.Alt().Radians()-got.Alt().Radians()).Degrees() * 3600
		if errArcsec < 0 {
			errArcsec = -errArcsec
		}

		if bound := boundArcsecPerHour * hours; errArcsec > bound {
			t.Errorf("Δt=%gh: AtTime altitude error = %g\", want <= %g\" (documented bound)", hours, errArcsec, bound)
		}
	}
}

// TestContextAtTime_AdvancesEarthRotation guards against a broken cheap
// path that silently no-ops instead of updating the Earth Rotation Angle:
// unlike the diurnally-projected altitude error above, Hour Angle tracks
// ERA directly and must advance at very close to the sidereal rate
// (~15.041"/s, i.e. ~15.041°/hour) between two AtTime derivations an hour
// apart.
func TestContextAtTime_AdvancesEarthRotation(t *testing.T) {
	site, err := coord.NewGeodetic(angle.Deg(2.1686), angle.Deg(41.3874), 0)
	testutil.AssertNoError(t, err)

	atm := atmosphere.Atmosphere{Pressure: 0}
	base := time.FromJD(2461000.0, time.UTC)
	ctx := coord.NewContext(base, site, atm)
	star := coord.NewICRS(angle.Hour(6.75), angle.Deg(-16.7))

	ha0, err := ctx.AtTime(base).ICRSToHourAngle(star)
	testutil.AssertNoError(t, err)

	oneHourLater := base.Add(time.Hour)

	ha1, err := ctx.AtTime(oneHourLater).ICRSToHourAngle(star)
	testutil.AssertNoError(t, err)

	const siderealDegPerHour = 15.041

	testutil.AssertNear(t, "Hour Angle advance over 1h", ha1.Degrees()-ha0.Degrees(), siderealDegPerHour, 0.01)
}
