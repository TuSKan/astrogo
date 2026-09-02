package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// equatorSite and a zero-pressure atmosphere are shared by every test in
// this file — none of them care about refraction, only the astrometry
// (ASTROM.V) Apco13 already computes.
func equatorSite(t *testing.T) *coord.Geodetic {
	t.Helper()

	site, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	testutil.AssertNoError(t, err)

	return site
}

var noRefraction = atmosphere.Refraction{Pressure: 0}

// TestBarycentricRVCorrection_BoundedByEarthPlusSiteSpeed checks
// |correction| never exceeds Earth's orbital speed (~29.8 km/s) plus a
// generous margin for the observing site's own diurnal rotation speed
// (<=~0.5 km/s at the equator, less elsewhere) — a real physical bound
// on BarycentricVelocity's magnitude, not a property of any one target.
// Sampled across a full year and several target directions so no
// particular geometry is assumed.
func TestBarycentricRVCorrection_BoundedByEarthPlusSiteSpeed(t *testing.T) {
	site := equatorSite(t)

	const maxPlausibleKmS = 31.0 // ~29.8 (orbital) + ~0.5 (equatorial rotation) + margin

	targets := []coord.ICRS{
		coord.NewICRS(angle.Zero(), angle.Zero()),
		coord.NewICRS(angle.Deg(90), angle.Zero()),
		coord.NewICRS(angle.Deg(180), angle.Zero()),
		coord.NewICRS(angle.Deg(270), angle.Zero()),
		coord.NewICRS(angle.Zero(), angle.Deg(89)),
		coord.NewICRS(angle.Zero(), angle.Deg(-89)),
	}

	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	for day := 0; day < 366; day += 15 {
		tm := base.AddDays(float64(day))
		ctx := coord.NewContext(tm, site, noRefraction)

		for _, target := range targets {
			corr := ctx.BarycentricRVCorrection(target)
			if math.Abs(corr) > maxPlausibleKmS {
				t.Errorf("day %d: |BarycentricRVCorrection| = %v km/s, exceeds plausible bound %v",
					day, corr, maxPlausibleKmS)
			}

			helioCorr, err := ctx.HeliocentricRVCorrection(target)
			testutil.AssertNoError(t, err)

			if math.Abs(helioCorr) > maxPlausibleKmS {
				t.Errorf("day %d: |HeliocentricRVCorrection| = %v km/s, exceeds plausible bound %v",
					day, helioCorr, maxPlausibleKmS)
			}
		}
	}
}

// TestBarycentricRVCorrection_AnnualSinusoid samples a target sitting
// exactly at the vernal equinox direction (RA=0, Dec=0) — the point
// where the ecliptic crosses the celestial equator, so it lies in
// Earth's own orbital plane — across a full year. Earth's orbital
// motion should carry the correction through a full sinusoidal cycle:
// a clear sign change and a peak-to-peak range close to twice Earth's
// orbital speed (~59.6 km/s), not a constant or monotonic value.
func TestBarycentricRVCorrection_AnnualSinusoid(t *testing.T) {
	site := equatorSite(t)
	target := coord.NewICRS(angle.Zero(), angle.Zero())
	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	var (
		lo, hi      = math.Inf(1), math.Inf(-1)
		sawPositive bool
		sawNegative bool
	)

	for day := 0; day < 366; day += 5 {
		tm := base.AddDays(float64(day))
		ctx := coord.NewContext(tm, site, noRefraction)

		corr := ctx.BarycentricRVCorrection(target)

		if corr < lo {
			lo = corr
		}

		if corr > hi {
			hi = corr
		}

		if corr > 5 {
			sawPositive = true
		}

		if corr < -5 {
			sawNegative = true
		}
	}

	if !sawPositive || !sawNegative {
		t.Errorf("expected a clear sign change across the year (min=%v max=%v), got sawPositive=%v sawNegative=%v",
			lo, hi, sawPositive, sawNegative)
	}

	peakToPeak := hi - lo
	if peakToPeak < 40 || peakToPeak > 65 {
		t.Errorf("peak-to-peak correction = %v km/s, want ~59.6 km/s (2x Earth's orbital speed), within [40, 65]", peakToPeak)
	}
}

// TestBarycentricRVCorrection_PerpendicularTargetIsZero constructs a
// target direction exactly perpendicular to the observer's own
// instantaneous barycentric velocity — by construction, the projection
// (and so the correction) must be zero, since a purely transverse
// (perpendicular) velocity component contributes nothing to a radial
// (line-of-sight) measurement.
func TestBarycentricRVCorrection_PerpendicularTargetIsZero(t *testing.T) {
	site := equatorSite(t)
	tm := time.Date(2026, time.March, 15, 6, 0, 0, 0, time.LocationUTC)
	ctx := coord.NewContext(tm, site, noRefraction)

	v := ctx.BarycentricVelocity()
	vUnit := v.Unit()

	// Any vector not parallel to vUnit, crossed with vUnit, gives a
	// vector perpendicular to vUnit. (0,0,1) is never parallel to a
	// barycentric velocity direction (which lies close to the ecliptic
	// plane), so this is safe without a parallelism check.
	perp := vUnit.Cross(vector.V3(0, 0, 1)).Unit()

	var target coord.ICRS
	target.FromUnitVector(perp)

	corr := ctx.BarycentricRVCorrection(target)
	testutil.AssertNear(t, "perpendicular-target correction", corr, 0, 1e-9)
}

// TestBarycentricRVCorrection_AntipodalTargetsFlipSign confirms that
// reversing a target's direction on the sky exactly negates the
// correction — a direct consequence of the dot-product projection: the
// same velocity component along an antiparallel direction has the
// opposite sign.
func TestBarycentricRVCorrection_AntipodalTargetsFlipSign(t *testing.T) {
	site := equatorSite(t)
	tm := time.Date(2026, time.June, 10, 18, 0, 0, 0, time.LocationUTC)
	ctx := coord.NewContext(tm, site, noRefraction)

	target := coord.NewICRS(angle.Deg(37), angle.Deg(-12))

	var antipodal coord.ICRS
	antipodal.FromUnitVector(target.ToUnitVector().MulScalar(-1))

	corr := ctx.BarycentricRVCorrection(target)
	antipodalCorr := ctx.BarycentricRVCorrection(antipodal)

	testutil.AssertNear(t, "antipodal correction", antipodalCorr, -corr, 1e-9)
}

// TestBarycentricRVCorrection_DiurnalAmplitudeScalesWithLatitude checks
// that the diurnal (site-rotation) contribution to the correction — the
// day-scale oscillation riding on top of Earth's near-constant orbital
// velocity over any single day — shrinks with the observing site's
// latitude, proportional to cos(latitude), since a site's own rotational
// speed about Earth's axis scales the same way.
func TestBarycentricRVCorrection_DiurnalAmplitudeScalesWithLatitude(t *testing.T) {
	target := coord.NewICRS(angle.Deg(120), angle.Deg(20))
	base := time.Date(2026, time.March, 15, 0, 0, 0, 0, time.LocationUTC)

	diurnalSwing := func(latDeg float64) float64 {
		site, err := coord.NewGeodetic(angle.Zero(), angle.Deg(latDeg), 0)
		testutil.AssertNoError(t, err)

		lo, hi := math.Inf(1), math.Inf(-1)

		for h := range 24 {
			tm := base.AddDays(float64(h) / 24.0)
			ctx := coord.NewContext(tm, site, noRefraction)

			corr := ctx.BarycentricRVCorrection(target)
			if corr < lo {
				lo = corr
			}

			if corr > hi {
				hi = corr
			}
		}

		return hi - lo
	}

	equatorSwing := diurnalSwing(0)
	highLatSwing := diurnalSwing(60)

	// Both swings are dominated by measurement/sampling noise unless
	// they're comfortably above zero — guard against a degenerate
	// near-zero equatorial swing before taking a ratio of it.
	if equatorSwing < 0.05 {
		t.Fatalf("equatorial diurnal swing = %v km/s, too small to compare ratios reliably", equatorSwing)
	}

	gotRatio := highLatSwing / equatorSwing
	wantRatio := math.Cos(60 * math.Pi / 180)

	testutil.AssertNear(t, "diurnal swing ratio (60 deg / equator)", gotRatio, wantRatio, 0.15)
}

// TestHeliocentricRVCorrection_DiffersFromBarycentric confirms the two
// corrections are genuinely different quantities (the Sun's own
// barycentric motion, dominated by Jupiter's ~12-year pull, is a real,
// nonzero effect on the order of ~10s of m/s) rather than
// HeliocentricRVCorrection accidentally being a copy of the barycentric
// one.
func TestHeliocentricRVCorrection_DiffersFromBarycentric(t *testing.T) {
	site := equatorSite(t)
	tm := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)
	ctx := coord.NewContext(tm, site, noRefraction)

	target := coord.NewICRS(angle.Deg(200), angle.Deg(30))

	bary := ctx.BarycentricRVCorrection(target)

	helio, err := ctx.HeliocentricRVCorrection(target)
	testutil.AssertNoError(t, err)

	if bary == helio {
		t.Error("HeliocentricRVCorrection exactly equals BarycentricRVCorrection — the Sun's own barycentric motion isn't being applied")
	}

	// The Sun's barycentric speed is a few tens of m/s at most (Jupiter
	// is the dominant perturber) — the two corrections should be close,
	// not wildly different.
	if math.Abs(bary-helio) > 0.1 {
		t.Errorf("|bary - helio| = %v km/s, expected a small (<0.1 km/s) difference from the Sun's own barycentric motion", math.Abs(bary-helio))
	}
}

// TestObservedRadialVelocity_RoundTripsWithBarycentricRVCorrection
// confirms ObservedRadialVelocity is exactly the algebraic inverse of
// BarycentricRVCorrection, per its own documented derivation
// (rvObserved = rvBarycentric - correction) — feeding a barycentric RV
// through ObservedRadialVelocity and the result back through
// BarycentricRVCorrection's own defining relation must recover the
// original value, for several distinct target directions and epochs so
// this isn't just true by coincidence for one geometry.
func TestObservedRadialVelocity_RoundTripsWithBarycentricRVCorrection(t *testing.T) {
	site := equatorSite(t)

	targets := []coord.ICRS{
		coord.NewICRS(angle.Zero(), angle.Zero()),
		coord.NewICRS(angle.Deg(90), angle.Deg(30)),
		coord.NewICRS(angle.Deg(200), angle.Deg(-45)),
		coord.NewICRS(angle.Deg(315), angle.Deg(60)),
	}

	epochs := []time.Time{
		time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC),
		time.Date(2026, time.July, 1, 0, 0, 0, 0, time.LocationUTC),
	}

	const rvBarycentric = -12.34

	for _, tm := range epochs {
		ctx := coord.NewContext(tm, site, noRefraction)

		for _, target := range targets {
			rvObserved, err := ctx.ObservedRadialVelocity(target, rvBarycentric)
			testutil.AssertNoError(t, err)

			// The two conversions are exact inverses, so this closes to
			// floating point rather than to a series truncation.
			roundTripped, err := ctx.BarycentricRadialVelocity(target, rvObserved)
			testutil.AssertNoError(t, err)

			testutil.AssertNear(t, "round-tripped barycentric RV", roundTripped, rvBarycentric, 1e-12)
		}
	}
}

// TestBarycentricRadialVelocity_ComposesRedshiftsMultiplicatively checks the
// conversion against the relation it is derived from, rather than against a
// restatement of its own arithmetic.
//
// A radial velocity is a redshift, and successive shifts compose as
// (1+z) = (1+z1)(1+z2)(1+z3). Two are Doppler — the target moving and the
// observer moving — and the third is the observer clock rate against a
// barycentric one, which is Context.ObserverFrameShift. Building the
// expected value that way — from the product, not from the expanded
// three-term form the implementation uses — means a sign slip or a dropped
// term in either expression fails the test.
//
// The tolerance is 1e-9 km/s, a micrometre per second, and it is set by the
// *reference* expression rather than by the code under test. Both velocities
// are of order 1e-4 c, so the bracket (1+a)(1+b)-1 subtracts two numbers that
// agree to fifteen digits and c multiplies what survives back up: the product
// form is good to about 3e-11 km/s and no better. The implementation's
// expanded form does not perform that subtraction at all, which makes it the
// more accurate of the two as well as the cheaper. That is why it is written
// out rather than expressed as the product it comes from.
func TestBarycentricRadialVelocity_ComposesRedshiftsMultiplicatively(t *testing.T) {
	site := equatorSite(t)
	ctx := coord.NewContext(
		time.Date(2026, time.January, 4, 0, 0, 0, 0, time.LocationUTC), site, noRefraction)

	// Speed of light in km/s, spelled out here so the test does not import
	// the same constant the implementation does.
	const c = 299792.458

	targets := []coord.ICRS{
		coord.NewICRS(angle.Zero(), angle.Zero()),
		coord.NewICRS(angle.Deg(90), angle.Deg(30)),
		coord.NewICRS(angle.Deg(200), angle.Deg(-45)),
	}

	// Spanning a Sun-like star, a thick-disc star and a halo star.
	for _, rvObserved := range []float64{0, -5.5, 20, 100, -300} {
		for _, target := range targets {
			corr := ctx.BarycentricRVCorrection(target)

			// Three shifts now, not two: the observer's own clock rate is
			// the third factor. Built here from the product, as the
			// definition, while the implementation expands it.
			shift, serr := ctx.ObserverFrameShift()
			testutil.AssertNoError(t, serr)

			want := c * ((1+rvObserved/c)*(1+corr/c)*(1+shift) - 1)

			got, err := ctx.BarycentricRadialVelocity(target, rvObserved)
			testutil.AssertNoError(t, err)

			testutil.AssertNear(t, "barycentric RV from the redshift product", got, want, 1e-9)
		}
	}
}

// TestBarycentricRadialVelocity_ExceedsTheAdditiveFormByTheDocumentedAmount
// measures what the old additive form dropped, because the doc comments now
// quote those figures and a quoted figure nobody checks is a comment.
//
// The gap is rvObserved*corr/c, so it grows with the target's own velocity
// and vanishes entirely at zero — which is why the 175-case Astropy fixture,
// whose every target has no radial velocity, could not see this.
func TestBarycentricRadialVelocity_ExceedsTheAdditiveFormByTheDocumentedAmount(t *testing.T) {
	site := equatorSite(t)

	// Early January, when Earth is near perihelion and moving fastest, and a
	// target on the ecliptic at RA 0 — the geometry that maximises the
	// correction and so the term that scales with it.
	ctx := coord.NewContext(
		time.Date(2026, time.January, 4, 0, 0, 0, 0, time.LocationUTC), site, noRefraction)
	target := coord.NewICRS(angle.Zero(), angle.Zero())

	corr := ctx.BarycentricRVCorrection(target)
	if math.Abs(corr) < 25 {
		t.Fatalf("correction is %.3f km/s, expected about 30 near perihelion on the ecliptic; "+
			"the geometry this test relies on has changed", corr)
	}

	const c = 299792.458

	cases := []struct {
		rvObserved float64
		what       string
	}{
		{0, "no radial velocity — the term vanishes, which is the fixture's blind spot"},
		{-5.5, "Sirius"},
		{20, "a typical thick-disc star"},
		{300, "a halo star"},
	}

	// The observer's own frame shift is now part of the answer too, and it
	// does not scale with the target's velocity — so it is separated here
	// rather than folded in, which keeps this test measuring the composition
	// term it was written for.
	shift, err := ctx.ObserverFrameShift()
	testutil.AssertNoError(t, err)

	for _, tc := range cases {
		additive := tc.rvObserved + corr

		exact, eerr := ctx.BarycentricRadialVelocity(target, tc.rvObserved)
		testutil.AssertNoError(t, eerr)

		// The frame shift acts on the whole classical value, not on c alone,
		// so subtracting shift*c leaves shift*rv behind — 4.8 mm/s for the
		// halo star, which is small and is not nothing. Subtracting the exact
		// contribution keeps this test measuring the composition term it was
		// written for rather than a mixture.
		classical := tc.rvObserved + corr + tc.rvObserved*corr/c
		frameMPerS := shift * (c + classical) * 1e3

		gapMPerS := (exact-additive)*1e3 - frameMPerS
		wantMPerS := tc.rvObserved * corr / c * 1e3

		t.Logf("rv %+7.1f km/s (%s): composition term %+8.2f m/s, frame shift %+.2f",
			tc.rvObserved, tc.what, gapMPerS, frameMPerS)

		testutil.AssertNear(t, "composition term against the additive form",
			gapMPerS, wantMPerS, 1e-9)
	}

	// The claim the doc comments make: at 46.6 km/s the dropped term equals
	// the 4.66 m/s of relativistic physics this package omits, so beyond it
	// the composition error dominates the modelling error.
	const crossover = 46.6

	dropped := math.Abs(crossover*corr/c) * 1e3
	if dropped < 4.0 || dropped > 5.4 {
		t.Errorf("at %.1f km/s the dropped term is %.2f m/s; the doc comments claim it reaches "+
			"the 4.66 m/s of omitted relativistic terms there", crossover, dropped)
	}
}

// TestTopocentricRadialVelocityIsTheLineOfSightComponent checks the
// projection against the geometry it is, with no ephemeris involved.
//
// Written because the function shipped with only a network test against JPL
// Horizons. That comparison is worth having and is the wrong thing to rely
// on: it needs an external service, it is invisible to an untagged coverage
// run, and when it fails it cannot say whether the projection, the frame or
// the ephemeris moved. These can.
func TestTopocentricRadialVelocityIsTheLineOfSightComponent(t *testing.T) {
	site := equatorSite(t)
	ctx := coord.NewContext(
		time.Date(2026, time.March, 15, 0, 0, 0, 0, time.LocationUTC), site, noRefraction)

	// Far enough that the observer's offset from the geocentre does not turn
	// the line of sight appreciably, so the geometry is the pure projection.
	const farAU = 100.0

	pos := vector.V3(farAU, 0, 0)

	// Straight away along the line of sight: the whole speed is radial.
	const auPerDay = 0.01 // ~17.3 km/s

	away := ctx.TopocentricRadialVelocity(pos, vector.V3(auPerDay, 0, 0))
	toward := ctx.TopocentricRadialVelocity(pos, vector.V3(-auPerDay, 0, 0))

	// The observer's own motion is common to both, so it cancels in the
	// difference and doubles in the sum. Halving the difference leaves the
	// body's speed along the line of sight, which for a body this far away
	// on the x-axis is its whole x-velocity.
	//
	// The first version of this test asserted the sum was zero, reasoning
	// that the site term "cancels". It does not: (v - s) + (-v - s) = -2s.
	// It came out at 0.125 km/s, which is twice a plausible site component,
	// and the arithmetic was the thing that was wrong.
	const kmPerSecPerAUPerDay = 149597870.7 / 86400.0

	radial := (away - toward) / 2
	testutil.AssertNear(t, "body speed recovered from the difference",
		radial, auPerDay*kmPerSecPerAUPerDay, 1e-6)

	// And the sum is twice the site's own component, so it is bounded by
	// twice the equatorial rotation speed.
	if sum := math.Abs(away + toward); sum > 2*0.4651 {
		t.Errorf("away plus toward is %.4f km/s, more than twice the site's rotation speed; "+
			"it should be exactly -2 times the observer's line-of-sight component", sum)
	}

	if away <= 0 {
		t.Errorf("a body moving directly away has radial velocity %v; it must be positive", away)
	}

	// Across the line of sight: nothing radial but the site's own motion, so
	// the result must be small rather than of order the body's speed.
	across := ctx.TopocentricRadialVelocity(pos, vector.V3(0, auPerDay, 0))
	if math.Abs(across) > 0.5 {
		t.Errorf("a body moving perpendicular to the line of sight has radial velocity "+
			"%v km/s; only the site's rotation should survive, which is under 0.47", across)
	}
}

// TestTopocentricRadialVelocityCarriesTheDiurnalTerm pins the part that is
// easy to leave out and impossible to notice: a body at rest relative to the
// geocentre still has a radial velocity, because the observer is moving.
//
// It is 0.465 km/s at the equator and nothing at the pole, and for the Moon
// it is the dominant term rather than a correction — the Moon's own
// geocentric radial velocity stays inside about 0.06 km/s.
func TestTopocentricRadialVelocityCarriesTheDiurnalTerm(t *testing.T) {
	at := time.Date(2026, time.March, 15, 6, 0, 0, 0, time.LocationUTC)

	// A body at rest relative to the geocentre. Whatever radial velocity it
	// shows is the observer's own.
	atRest := vector.Zero()
	pos := vector.V3(100, 0, 0)

	swing := func(latDeg float64) float64 {
		site, err := coord.NewGeodetic(angle.Zero(), angle.Deg(latDeg), 0)
		testutil.AssertNoError(t, err)

		lo, hi := math.Inf(1), math.Inf(-1)

		for h := range 24 {
			ctx := coord.NewContext(at.AddDays(float64(h)/24), site, noRefraction)

			rv := ctx.TopocentricRadialVelocity(pos, atRest)
			lo, hi = math.Min(lo, rv), math.Max(hi, rv)
		}

		return hi - lo
	}

	equator := swing(0)

	// Twice the equatorial rotation speed, since the site swings toward the
	// body and away from it over a day.
	const equatorialSpeed = 0.4651

	if equator < 1.5*equatorialSpeed || equator > 2.5*equatorialSpeed {
		t.Errorf("the diurnal swing at the equator is %.4f km/s, want about %.4f — twice the "+
			"site's rotation speed", equator, 2*equatorialSpeed)
	}

	// The term scales as the cosine of latitude, so a pole barely moves.
	if pole := swing(89.9); pole > 0.05*equator {
		t.Errorf("the diurnal swing at the pole is %.4f km/s against %.4f at the equator; "+
			"it should very nearly vanish", pole, equator)
	}
}

// TestTopocentricRadialVelocityHandlesAZeroLineOfSight covers the guard, for
// a body at the observer's own position. The direction is undefined there and
// a naive unit vector would be NaN, which would propagate silently into a
// schedule rather than failing.
func TestTopocentricRadialVelocityHandlesAZeroLineOfSight(t *testing.T) {
	site := equatorSite(t)
	ctx := coord.NewContext(
		time.Date(2026, time.March, 15, 0, 0, 0, 0, time.LocationUTC), site, noRefraction)

	rv := ctx.TopocentricRadialVelocity(ctx.ObsVec(), vector.V3(0.01, 0, 0))
	if rv != 0 {
		t.Errorf("a body at the observer's own position gave %v, want 0", rv)
	}
}
