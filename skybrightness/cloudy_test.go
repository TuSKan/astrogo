package skybrightness_test

import (
	"errors"
	"math"
	"testing"
	gotime "time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/unit"
)

// cloudyCity is a Garstang-shaped source, with the Q and q of
// Kocifaj (2007) §3, built on the same geometry helper the clear-sky tests
// use so the two components are exercised against comparable inventories.
func cloudyCity(tb testing.TB, bearingDeg, distanceKM float64) *skybrightness.UniformEmitter {
	tb.Helper()

	e := cityAt(tb, bearingDeg, distanceKM, 1e-3)
	e.Emission = skybrightness.GarstangEmission{
		ReflectedFraction: 0.15,
		DirectFraction:    0.15,
	}

	return e
}

// cloudyScene builds an atmosphere with or without an overcast deck, at the
// aerosol of Kocifaj (2007) §3: a reference AOD of 0.4 at 500 nm, Angstrom
// 1.3, single-scattering albedo 0.85 and asymmetry 0.65.
//
// The boundary-layer height is 1538 m, which is 1/beta for the paper's
// beta = 0.65 km^-1 — the same profile written the way this package
// parameterises it.
func cloudyScene(tb testing.TB, baseM, albedo float64) *skybrightness.Scene {
	tb.Helper()

	b := atmosphere.NewBuilder().
		Surface(1013, 288).
		Aerosol(0.4, 500, 1.3, 0.85, 0.65).
		BoundaryLayer(1538)

	if baseM > 0 {
		b = b.AddCloud(atmosphere.CloudLayer{
			Fraction:     1,
			BaseAlt:      unit.AltitudeM(baseM),
			TopAlt:       unit.AltitudeM(baseM + 500),
			Albedo:       unit.SpectralAlbedo(albedo),
			OpticalDepth: 20,
		})
	}

	air, err := b.Build()
	if err != nil {
		tb.Fatalf("atmosphere Build: %v", err)
	}

	observer, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2000)
	if err != nil {
		tb.Fatalf("NewGeodetic: %v", err)
	}

	return &skybrightness.Scene{
		Observer:   observer,
		Time:       gotime.Date(2026, 3, 20, 3, 0, 0, 0, gotime.UTC),
		Atmosphere: air,
	}
}

// An overcast deck over a city brightens the sky beneath it.
//
// # Why this is the test that matters
//
// It is the whole reason the component exists. A cloud returns light that
// would otherwise have left the atmosphere, so the sky above a lit city gets
// brighter rather than darker — the opposite of the intuition that clouds
// block light, and the thing a model that multiplied a clear-sky answer by a
// transmission could never produce. Kocifaj, Falchi & Kundracik (2025) report
// more than fifteenfold at the zenith over their simulated city.
func TestCloudBrightensTheSkyOverASource(t *testing.T) {
	t.Parallel()

	// The observer stands close to the city, so the cloud overhead is lit
	// from below.
	city := cloudyCity(t, 0, 2)

	c, err := skybrightness.NewCloudySkyglow([]skybrightness.GroundEmitter{city})
	if err != nil {
		t.Fatalf("NewCloudySkyglow: %v", err)
	}

	clearSky := radianceAt(t, c, cloudyScene(t, 0, 0), 90, 0)
	overcast := radianceAt(t, c, cloudyScene(t, 1000, 0.7), 90, 0)

	if clearSky <= 0 {
		t.Fatalf("the clearSky sky is %.4g, which is not a sky", clearSky)
	}

	if overcast <= clearSky {
		t.Fatalf("overcast %.4g is not brighter than clearSky %.4g over a city; the cloud "+
			"term must return light that would otherwise have escaped", overcast, clearSky)
	}

	t.Logf("amplification over the city at the zenith: %.1fx", overcast/clearSky)
}

// The cloud term scales with reflectance, and with height in a way that
// depends on where the source is.
//
// # The height behaviour is not simply 1/H^2
//
// The term carries 1/H^2, so a lower deck ought to be brighter, and directly
// over a city it is. But it also carries cos^4(z0_H), the angle at which the
// city sees the patch of cloud being looked at, and for a source off to one
// side raising the deck improves that angle faster than the inverse square
// costs: a low cloud overhead is seen edge-on from a city two kilometres away
// and lit weakly, while a higher one is closer to face-on.
//
// Measured here, with the observer at the zenith: a source 500 m away makes
// the 1 km deck about five times brighter than the 3 km one, and a source
// 2 km away reverses it. Asserting "lower is always brighter" would have been
// asserting one half of that competition — and it is the half a test written
// from intuition picks.
func TestCloudTermScalesWithReflectanceAndHeight(t *testing.T) {
	t.Parallel()

	near := cloudyCity(t, 0, 0.5)
	offset := cloudyCity(t, 0, 2)

	nearC, err := skybrightness.NewCloudySkyglow([]skybrightness.GroundEmitter{near})
	if err != nil {
		t.Fatalf("NewCloudySkyglow: %v", err)
	}

	offsetC, err := skybrightness.NewCloudySkyglow([]skybrightness.GroundEmitter{offset})
	if err != nil {
		t.Fatalf("NewCloudySkyglow: %v", err)
	}

	// Linear in reflectance, at the paper's own two values.
	dim := radianceAt(t, nearC, cloudyScene(t, 1000, 0.4), 90, 0)
	bright := radianceAt(t, nearC, cloudyScene(t, 1000, 0.7), 90, 0)

	if bright <= dim {
		t.Errorf("rho = 0.7 gives %.4g and rho = 0.4 gives %.4g; the cloud term is "+
			"linear in reflectance", bright, dim)
	}

	// Nearly overhead: the inverse square wins, at the paper's two altitudes.
	lowNear := radianceAt(t, nearC, cloudyScene(t, 1000, 0.7), 90, 0)
	highNear := radianceAt(t, nearC, cloudyScene(t, 3000, 0.7), 90, 0)

	if lowNear <= highNear {
		t.Errorf("over a source 500 m away a 1 km deck gives %.4g and a 3 km deck %.4g; "+
			"with the patch nearly above the source, 1/H^2 should dominate",
			lowNear, highNear)
	}

	// Offset: the illumination geometry wins instead.
	lowOffset := radianceAt(t, offsetC, cloudyScene(t, 1000, 0.7), 90, 0)
	highOffset := radianceAt(t, offsetC, cloudyScene(t, 3000, 0.7), 90, 0)

	if highOffset <= lowOffset {
		t.Errorf("over a source 2 km away a 3 km deck gives %.4g and a 1 km deck %.4g; "+
			"the higher patch is lit closer to face-on and should win",
			highOffset, lowOffset)
	}

	t.Logf("500 m source: 1 km deck is %.2fx the 3 km deck", lowNear/highNear)
	t.Logf("2 km source:  3 km deck is %.2fx the 1 km deck", highOffset/lowOffset)
}

// Radiance falls steeply with angular distance from the source.
//
// Kocifaj (2007) §3.1 states it for the cloudless case: "reduction of the
// intensity may reach two orders of magnitude" across the sky. This checks
// the direction and the rough scale, not a digitised curve — the figures are
// in relative units on a logarithmic scale and reading numbers off them would
// be inventing precision.
func TestCloudlessRadianceFallsAwayFromTheSource(t *testing.T) {
	t.Parallel()

	// Far enough that the sky above the source and the sky away from it are
	// genuinely different lines of sight.
	city := cloudyCity(t, 0, 10)

	c, err := skybrightness.NewCloudySkyglow([]skybrightness.GroundEmitter{city})
	if err != nil {
		t.Fatalf("NewCloudySkyglow: %v", err)
	}

	scene := cloudyScene(t, 0, 0)

	toward := radianceAt(t, c, scene, 30, 0) // low, toward the city
	away := radianceAt(t, c, scene, 30, 180) // low, directly away
	zenith := radianceAt(t, c, scene, 90, 0) // overhead

	if toward <= zenith {
		t.Errorf("toward the city at 30 degrees %.4g is not above the zenith %.4g", toward, zenith)
	}

	if zenith <= away {
		t.Errorf("the zenith %.4g is not above the anti-source direction %.4g", zenith, away)
	}

	ratio := toward / away

	t.Logf("toward/away at 30 degrees altitude: %.1fx", ratio)

	if ratio < 3 {
		t.Errorf("the sky is only %.2fx brighter toward the source than away from it; the "+
			"paper describes a steep gradation across the sky", ratio)
	}
}

// Fractional cover is refused rather than answered.
//
// Returning the overcast result for a tenth of a sky's cover would be wrong
// by an order of magnitude in the direction that flatters the model, so this
// is a case where declining is the honest answer.
func TestPartialCloudIsRefused(t *testing.T) {
	t.Parallel()

	city := cloudyCity(t, 0, 5)

	c, err := skybrightness.NewCloudySkyglow([]skybrightness.GroundEmitter{city})
	if err != nil {
		t.Fatalf("NewCloudySkyglow: %v", err)
	}

	air, err := atmosphere.NewBuilder().
		Surface(1013, 288).
		Aerosol(0.4, 500, 1.3, 0.85, 0.65).
		BoundaryLayer(1538).
		AddCloud(atmosphere.CloudLayer{
			Fraction: 0.5,
			BaseAlt:  1000,
			TopAlt:   1500,
			Albedo:   0.6,
		}).
		Build()
	if err != nil {
		t.Fatalf("atmosphere Build: %v", err)
	}

	scene := cloudyScene(t, 0, 0)
	scene.Atmosphere = air

	grid := skybrightness.DefaultOpticalGrid()
	dst := skybrightness.NewSpectralRadiance(grid)

	_, err = c.AddRadiance(t.Context(), dst, grid,
		coord.NewAltAz(angle.Deg(90), angle.Deg(0)), scene)
	if !errors.Is(err, skybrightness.ErrPartialCloud) {
		t.Fatalf("got %v, want ErrPartialCloud", err)
	}
}

// The height integral has converged at the default resolution.
//
// A quadrature that has not converged produces a number that looks like a
// radiance and moves when nothing physical has changed, which is the failure
// this catches. Doubling the steps must not move the answer meaningfully.
func TestHeightIntegralHasConverged(t *testing.T) {
	t.Parallel()

	city := cloudyCity(t, 0, 5)

	scene := cloudyScene(t, 1000, 0.7)

	coarse, err := skybrightness.NewCloudySkyglow(
		[]skybrightness.GroundEmitter{city},
		skybrightness.WithCloudySteps(skybrightness.DefaultCloudySteps))
	if err != nil {
		t.Fatalf("NewCloudySkyglow: %v", err)
	}

	fine, err := skybrightness.NewCloudySkyglow(
		[]skybrightness.GroundEmitter{city},
		skybrightness.WithCloudySteps(8*skybrightness.DefaultCloudySteps))
	if err != nil {
		t.Fatalf("NewCloudySkyglow: %v", err)
	}

	for _, alt := range []float64{90, 60, 30} {
		a := radianceAt(t, coarse, scene, alt, 0)
		b := radianceAt(t, fine, scene, alt, 0)

		if rel := math.Abs(a-b) / b; rel > 0.01 {
			t.Errorf("at %g degrees the default resolution gives %.6g and eight times "+
				"finer gives %.6g, a relative difference of %.3g", alt, a, b, rel)
		}
	}
}

// Two identical cities contribute more than one, and sources add linearly.
//
// Eq. 28 sums over sources, so this is the property that makes an inventory
// meaningful rather than a single aggregated number.
func TestSourcesAddLinearly(t *testing.T) {
	t.Parallel()

	scene := cloudyScene(t, 1000, 0.7)

	north := cloudyCity(t, 0, 5)
	south := cloudyCity(t, 180, 5)

	one, err := skybrightness.NewCloudySkyglow([]skybrightness.GroundEmitter{north})
	if err != nil {
		t.Fatalf("NewCloudySkyglow: %v", err)
	}

	both, err := skybrightness.NewCloudySkyglow([]skybrightness.GroundEmitter{north, south})
	if err != nil {
		t.Fatalf("NewCloudySkyglow: %v", err)
	}

	other, err := skybrightness.NewCloudySkyglow([]skybrightness.GroundEmitter{south})
	if err != nil {
		t.Fatalf("NewCloudySkyglow: %v", err)
	}

	a := radianceAt(t, one, scene, 90, 0)
	b := radianceAt(t, other, scene, 90, 0)
	sum := radianceAt(t, both, scene, 90, 0)

	if rel := math.Abs(sum-(a+b)) / sum; rel > 1e-12 {
		t.Errorf("two sources give %.9g and the sum of each alone is %.9g", sum, a+b)
	}
}

// Below the horizon there is no sky.
func TestNoRadianceBelowTheHorizon(t *testing.T) {
	t.Parallel()

	city := cloudyCity(t, 0, 5)

	c, err := skybrightness.NewCloudySkyglow([]skybrightness.GroundEmitter{city})
	if err != nil {
		t.Fatalf("NewCloudySkyglow: %v", err)
	}

	if got := radianceAt(t, c, cloudyScene(t, 1000, 0.7), -5, 0); got != 0 {
		t.Errorf("below the horizon the component wrote %.4g", got)
	}
}

// It shares an ID with the clear-sky solution, so a model cannot hold both.
//
// They compute the same physical contribution by different solutions, and
// summing them would double-count artificial light entirely.
func TestCloudyAndClearSkyglowCannotBothBeRegistered(t *testing.T) {
	t.Parallel()

	city := cloudyCity(t, 0, 5)

	cloudy, err := skybrightness.NewCloudySkyglow([]skybrightness.GroundEmitter{city})
	if err != nil {
		t.Fatalf("NewCloudySkyglow: %v", err)
	}

	clearSky, err := skybrightness.NewArtificialSkyglow([]skybrightness.GroundEmitter{city})
	if err != nil {
		t.Fatalf("NewArtificialSkyglow: %v", err)
	}

	if cloudy.ID() != clearSky.ID() {
		t.Fatalf("the two solutions report different IDs, %q and %q; they compute the "+
			"same contribution and must collide", cloudy.ID(), clearSky.ID())
	}

	if _, err := skybrightness.NewModel("both", cloudy, clearSky); !errors.Is(
		err, skybrightness.ErrDuplicateComponent) {
		t.Fatalf("a model holding both was accepted: %v", err)
	}
}
