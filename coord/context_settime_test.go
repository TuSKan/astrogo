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

// settimeBoundArcsec is SetTime's documented bound against NewContext at the
// same instant: linear interpolation of the series' state across a day,
// limited by nutation's 13.66-day term (see SetTime). Measured ≤0.0049″ over
// 30 days at three latitudes and two epochs.
const settimeBoundArcsec = 0.007

// TestSetTime_IsNewContextAtANode: where SetTime evaluated the series, it is
// what NewContext builds there — at the instant ctx was built, and at the far
// end of the bracket a later move evaluated. Interpolation is exact at its
// ends, and the decomposition (series, then SOFA's Apco at t's Earth rotation
// angle) is NewContext's own.
func TestSetTime_IsNewContextAtANode(t *testing.T) {
	site, atm, base, star := setTimeFixture(t)

	ctx := coord.NewContext(base, site, atm)
	ctx.SetTime(base.Add(unit.Hours(12))) // evaluates the bracket's far end, a day out

	for _, at := range []time.Time{base.Add(unit.Days(1)), base} {
		ctx.SetTime(at)

		if d := sameAsNewContext(t, ctx, site, atm, star); d > 1e-12 {
			t.Errorf("%v: SetTime to a node is %g° from NewContext, want 1e-12", at, d)
		}
	}
}

// TestSetTime_StaysWithinItsBound walks a Context three days forward and two
// back in 37-minute steps, through holding, a first bracket, moving brackets
// and a rebuild, and holds every step to NewContext at the documented bound.
// It used to drift to 0.07″ before an hourly rebuild, most of it the
// observer's velocity held at the epoch's Earth rotation angle (#790).
//
// And at an instant between nodes the answer must differ from NewContext's:
// identical everywhere would mean SetTime evaluated the series at every
// step, which is right but is the cost it exists to avoid.
func TestSetTime_StaysWithinItsBound(t *testing.T) {
	site, atm, base, star := setTimeFixture(t)

	ctx := coord.NewContext(base, site, atm)

	var worst float64

	step := unit.Minutes(37)
	for i := 1; i <= 117; i++ { // three days out
		ctx.SetTime(base.Add(step * unit.Duration(i)))
		worst = max(worst, sameAsNewContext(t, ctx, site, atm, star)*3600)
	}

	for i := 117; i >= 39; i-- { // two days back
		ctx.SetTime(base.Add(step * unit.Duration(i)))
		worst = max(worst, sameAsNewContext(t, ctx, site, atm, star)*3600)
	}

	if worst > settimeBoundArcsec {
		t.Errorf("SetTime is %.4g″ from NewContext at worst, want <= %g″", worst, settimeBoundArcsec)
	}

	ctx.SetTime(base.Add(unit.Hours(36))) // mid-bracket

	// A rebuild through SetTime's own decomposition differs from NewContext
	// by up to the 1e-12° TestSetTime_IsNewContextAtANode allows, 3.6e-9″;
	// interpolation, by milliarcseconds mid-bracket (#731).
	if d := sameAsNewContext(t, ctx, site, atm, star) * 3600; d < 1e-6 {
		t.Errorf("mid-bracket, SetTime is %g″ from NewContext: it evaluated the series rather than interpolating", d)
	}
}

// TestSetTime_RebuildsFarAway: a move beyond the bracket next to the one ctx
// has evaluates the series at t, as NewContext does, in either direction, and
// works from there — so a jump costs one evaluation however far it goes.
func TestSetTime_RebuildsFarAway(t *testing.T) {
	site, atm, base, star := setTimeFixture(t)

	ctx := coord.NewContext(base, site, atm)

	for _, days := range []float64{5, 40, -3} {
		ctx.SetTime(base.Add(unit.Days(days)))

		if d := sameAsNewContext(t, ctx, site, atm, star); d > 1e-12 {
			t.Errorf("Δt=%g d: SetTime is %g° from NewContext, want a rebuild (1e-12)", days, d)
		}
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
// epoch for all eight hours, and drifted to ≲0.8″ (#675).
func TestSetTime_StepsANightAsNewContextDoes(t *testing.T) {
	site, atm, base, star := setTimeFixture(t)

	ctx := coord.NewContext(base, site, atm)

	var worst float64

	for i := 1; i <= 96; i++ {
		ctx.SetTime(base.Add(unit.Minutes(5 * float64(i))))
		worst = max(worst, sameAsNewContext(t, ctx, site, atm, star)*3600)
	}

	if worst > settimeBoundArcsec {
		t.Errorf("over 8 h in 5 min steps, SetTime is %g″ from NewContext at worst, want <= %g″",
			worst, settimeBoundArcsec)
	}
}
