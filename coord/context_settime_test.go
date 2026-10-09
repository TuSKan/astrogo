package coord_test

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// setTimeFixture is the site, atmosphere, epoch and star the SetTime tests
// share: Barcelona, no refraction, 2025-11-17.
func setTimeFixture(t *testing.T) (*coord.Geodetic, atmosphere.Refraction, time.Time, coord.ICRS) {
	t.Helper()

	site, err := coord.NewGeodetic(angle.Deg(2.1686), angle.Deg(41.3874), 0)
	testutil.AssertNoError(t, err)

	return site, atmosphere.Refraction{Pressure: 0}, time.FromJD(2461000.0, time.UTC),
		coord.NewICRS(angle.Hour(6.75), angle.Deg(-16.7))
}

// sameAsNewContext reports how far ctx's reductions are from a fresh
// NewContext at ctx's own instant, in degrees: the largest difference over
// the stellar path, the hour angle and the vector path.
func sameAsNewContext(t *testing.T, ctx *coord.Context, site *coord.Geodetic, atm atmosphere.Refraction, star coord.ICRS) float64 {
	t.Helper()

	fresh := coord.NewContext(ctx.Time(), site, atm)

	var worst float64

	note := func(a, b float64) {
		if d := a - b; d > worst {
			worst = d
		} else if -d > worst {
			worst = -d
		}
	}

	want, err := fresh.ICRSToAltAz(star)
	testutil.AssertNoError(t, err)

	got, err := ctx.ICRSToAltAz(star)
	testutil.AssertNoError(t, err)

	note(got.Alt().Degrees(), want.Alt().Degrees())
	note(got.Az().Degrees(), want.Az().Degrees())

	wantHA, err := fresh.ICRSToHourAngle(star)
	testutil.AssertNoError(t, err)

	gotHA, err := ctx.ICRSToHourAngle(star)
	testutil.AssertNoError(t, err)

	note(gotHA.Degrees(), wantHA.Degrees())

	// Moving-body path: an arbitrary geocentric ICRS vector at roughly
	// lunar distance (AU).
	vec := vector.V3(0.0016, 0.0019, 0.0008)
	wantGeo, gotGeo := fresh.GeocentricToObserved(vec), ctx.GeocentricToObserved(vec)
	note(gotGeo.Alt().Degrees(), wantGeo.Alt().Degrees())
	note(gotGeo.Az().Degrees(), wantGeo.Az().Degrees())

	return worst
}

// TestSetTime_ZeroDeltaMatchesNewContext proves SetTime(ctx.Time())
// reproduces a fresh NewContext exactly — the decomposition (the
// precession-nutation matrix, Era00 and Pom00 composed via C2tcio,
// astrom.Eral updated via Aper) must be bit-identical to the monolithic build
// at the same instant.
func TestSetTime_ZeroDeltaMatchesNewContext(t *testing.T) {
	site, atm, base, star := setTimeFixture(t)

	ctx := coord.NewContext(base, site, atm)
	ctx.SetTime(base)

	if d := sameAsNewContext(t, ctx, site, atm, star); d > 1e-12 {
		t.Errorf("SetTime to its own epoch is %g° from NewContext, want 1e-12", d)
	}
}

// TestSetTime_WithinTheHourIsTheCheapPath holds SetTime's error inside the
// hour to its documented ≲0.1″ per hour, and requires it to be non-zero: a
// result identical to a fresh NewContext means SetTime rebuilt, and the cheap
// path is not being tested at all.
func TestSetTime_WithinTheHourIsTheCheapPath(t *testing.T) {
	site, atm, base, star := setTimeFixture(t)

	const boundArcsecPerHour = 0.1

	for _, hours := range []float64{0.25, 0.5, 1} {
		ctx := coord.NewContext(base, site, atm)
		ctx.SetTime(base.Add(unit.Hours(hours)))

		d := sameAsNewContext(t, ctx, site, atm, star) * 3600
		if bound := boundArcsecPerHour * hours; d > bound {
			t.Errorf("Δt=%gh: SetTime is %g″ from NewContext, want <= %g″ (documented bound)", hours, d, bound)
		}

		if d == 0 {
			t.Errorf("Δt=%gh: SetTime matches NewContext exactly, so it rebuilt inside the hour", hours)
		}
	}
}

// TestSetTime_RebuildsAnHourOut is what SetTime adds to AtTime: past the
// hour it builds the Context again for t, so it matches NewContext exactly,
// in either direction, and then works from the new epoch. AtTime held the
// first epoch forever, and a whole night stepped from one root drifted to
// ≲0.8″ (#675).
func TestSetTime_RebuildsAnHourOut(t *testing.T) {
	site, atm, base, star := setTimeFixture(t)

	ctx := coord.NewContext(base, site, atm)

	for _, hours := range []float64{6, 24, -3} {
		ctx.SetTime(base.Add(unit.Hours(hours)))

		if d := sameAsNewContext(t, ctx, site, atm, star); d > 1e-12 {
			t.Errorf("Δt=%gh: SetTime is %g° from NewContext, want a rebuild (1e-12)", hours, d)
		}
	}

	// Half an hour from the last epoch, −3 h: the cheap path again.
	ctx.SetTime(base.Add(unit.Hours(-2.5)))

	if d := sameAsNewContext(t, ctx, site, atm, star); d == 0 {
		t.Error("half an hour from the rebuilt epoch, SetTime matches NewContext exactly: it rebuilt again")
	}
}

// TestSetTime_AdvancesEarthRotation guards against a cheap path that
// silently no-ops instead of updating the Earth Rotation Angle: Hour Angle
// tracks ERA directly and must advance at very close to the sidereal rate
// (~15.041°/hour) over an hour.
func TestSetTime_AdvancesEarthRotation(t *testing.T) {
	site, atm, base, star := setTimeFixture(t)

	ctx := coord.NewContext(base, site, atm)

	ha0, err := ctx.ICRSToHourAngle(star)
	testutil.AssertNoError(t, err)

	ctx.SetTime(base.Add(unit.Hours(1)))

	ha1, err := ctx.ICRSToHourAngle(star)
	testutil.AssertNoError(t, err)

	const siderealDegPerHour = 15.041

	testutil.AssertNear(t, "Hour Angle advance over 1h", ha1.Degrees()-ha0.Degrees(), siderealDegPerHour, 0.01)
}

// TestContextIsAValue is the copy SetTime's doc offers a caller that needs
// two instants at once: c := *ctx is independent, so moving it leaves ctx
// where it was.
func TestContextIsAValue(t *testing.T) {
	site, atm, base, star := setTimeFixture(t)

	ctx := coord.NewContext(base, site, atm)

	before, err := ctx.ICRSToAltAz(star)
	testutil.AssertNoError(t, err)

	c := *ctx
	c.SetTime(base.Add(unit.Hours(0.5)))
	c.SetTime(base.Add(unit.Hours(6)))

	if !ctx.Time().Equal(base) {
		t.Errorf("moving a copy moved the original to %v", ctx.Time())
	}

	after, err := ctx.ICRSToAltAz(star)
	testutil.AssertNoError(t, err)

	if after.Alt() != before.Alt() || after.Az() != before.Az() {
		t.Errorf("moving a copy changed the original's reduction: %v → %v", before, after)
	}
}

// TestSetTime_StepsANightAsNewContextDoes sweeps the night the way #675's
// samplers do, one Context through 96 five-minute steps, and holds every
// step to a fresh NewContext at the documented bound. AtTime held the first
// epoch for all eight hours; SetTime rebuilds each hour, so no step is ever
// more than an hour from its epoch.
func TestSetTime_StepsANightAsNewContextDoes(t *testing.T) {
	site, atm, base, star := setTimeFixture(t)

	ctx := coord.NewContext(base, site, atm)

	const boundArcsec = 0.1 // an hour's worth: the most any step can be from its epoch

	var worst float64

	for i := 1; i <= 96; i++ {
		ctx.SetTime(base.Add(unit.Minutes(5 * float64(i))))
		worst = max(worst, sameAsNewContext(t, ctx, site, atm, star)*3600)
	}

	if worst > boundArcsec {
		t.Errorf("over 8 h in 5 min steps, SetTime is %g″ from NewContext at worst, want <= %g″", worst, boundArcsec)
	}
}
