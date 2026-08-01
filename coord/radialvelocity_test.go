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

var noRefraction = atmosphere.Atmosphere{Pressure: 0}

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
