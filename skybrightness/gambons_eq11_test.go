//go:build validation

package skybrightness_test

import (
	"context"
	"math"
	"testing"
	gotime "time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/skybrightness/dataset/starlight"
	"github.com/TuSKan/astrogo/unit"
)

// Does the full scattering model close the Table 2 starlight-to-zodiacal gap?
//
// # The question
//
// Our zenith starlight/zodiacal ratio is 1.236 against Table 2's 1.075. The
// effective-optical-depth transfer cannot be responsible, because it is a
// common factor and a common factor cancels in a ratio. Table 2 comes from the
// paper's full model, so the one candidate that could account for it is the
// term the simplification stands in for: light scattered into the beam from
// the rest of the sky, which weights each component by its own distribution
// and so does not cancel.
//
// An earlier estimate of its size used the hemisphere-mean-to-zenith ratio of
// each component as a proxy for how much scattered light it supplies, and put
// the effect at about a quarter of the gap. That proxy weighted every
// direction equally. The kernel does not: measured in
// TestScatteredInIsNotACommonFactor, the overhead third of the sky delivers
// four and a half times what the horizon third does to a zenith sightline, for
// the same flux. So the proxy was wrong and the earlier figure with it. This
// test runs the integral itself.
//
// # The comparison
//
// Masana et al. Eq. 8 is L_obs = L_d + L_s, the direct and the scattered
// radiance. Two scenes make both readable through the public API:
//
//   - kappa = 0.5 is the web simplification, where the components' own
//     attenuation already stands in for the scattered term.
//   - kappa = 1 makes the components apply the true extinction and nothing
//     else, which is L_d exactly. Adding ScatteredIn to it gives Eq. 8.
//
// Comparing the ratio under both against Table 2's is what says whether the
// missing term is the explanation, part of it, or none of it.
func TestEq11AgainstTheTable2Ratio(t *testing.T) {
	testutil.RequireReachable(t, "github.com:443")

	ctx, cancel := context.WithTimeout(context.Background(), 40*gotime.Minute)
	defer cancel()

	remote.EnableDownloads(32<<20, remote.GaiaStarMap)

	// A grid over Johnson V alone, at 2 nm. The integral costs one field evaluation per
	// source direction per epoch, so the wavelength axis is the one place a
	// factor of twenty is free: the quantity under test is a band ratio and
	// nothing outside V enters it.
	grid, err := unit.NewSpectralGrid(495, 2, 58)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	band := johnsonVFromTable1()

	site, err := coord.NewGeodetic(angle.Deg(0), angle.Deg(table2LatDeg), table2ElevM)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	build := func(kappa float64) *atmosphere.Atmosphere {
		t.Helper()

		atm, err := atmosphere.NewBuilder().
			Surface(1013, 288).
			Aerosol(table2AOD550, 550, table2Angstrom, table2AerosolW, table2Asym).
			BoundaryLayer(1000).
			DiffuseScattering(kappa).
			Build()
		if err != nil {
			t.Fatalf("atmosphere Build at kappa %g: %v", kappa, err)
		}

		return atm
	}

	// kappa = 1 leaves the true extinction and no stand-in for scattering,
	// which is the L_d of Eq. 8.
	web, full := build(0.5), build(1.0)

	provider := eph.Default()

	epochs := table2Epochs(t, site, provider, web)
	if len(epochs) == 0 {
		t.Fatal("no astronomical-night epochs were found")
	}

	// The same epochs and the same zenith cap as TestAgainstGAMBONSTable2,
	// deliberately. A first version of this test took every eighth epoch to
	// save time and measured a starlight-to-zodiacal ratio of 0.48 where the
	// other test measures 1.24 — zodiacal light swings by a factor of several
	// as the ecliptic passes through the zenith over a year, so a subsample is
	// not a smaller version of the average, it is a different quantity. The
	// cross-check below now fails if the two tests ever drift apart again.
	capDirs := zenithCap(table2CapDeg, table2CapSamples)

	skyMap, err := starlight.Open(ctx)
	if err != nil {
		t.Skipf("could not fetch the published star map: %v", err)
	}

	stars, err := skyMap.Band("V")
	if err != nil {
		t.Fatalf("Band: %v", err)
	}

	isl, err := skybrightness.NewIntegratedStarlight(stars, solarLikeShape(grid), grid, band)
	if err != nil {
		t.Fatalf("NewIntegratedStarlight: %v", err)
	}

	// One model per component, because the scattered-in term has to be
	// attributed to the component that supplied the light. A single model
	// would give their sum and the ratio would be unrecoverable.
	type part struct {
		id    skybrightness.ComponentID
		model *skybrightness.Model
	}

	islModel, err := skybrightness.NewModel("eq11-starlight", isl)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	zodiModel, err := skybrightness.NewModel("eq11-zodiacal", skybrightness.NewZodiacalLight())
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	parts := []part{
		{skybrightness.Starlight, islModel},
		{skybrightness.Zodiacal, zodiModel},
	}

	type totals struct{ kappa, direct, scattered float64 }

	got := map[skybrightness.ComponentID]*totals{
		skybrightness.Starlight: {},
		skybrightness.Zodiacal:  {},
	}

	const rings = 10

	for _, when := range epochs {
		webScene := &skybrightness.Scene{
			Observer: site, Time: when, Atmosphere: web, Ephemeris: provider,
		}
		fullScene := &skybrightness.Scene{
			Observer: site, Time: when, Atmosphere: full, Ephemeris: provider,
		}

		for _, view := range capDirs {
			for _, p := range parts {
				// The web model: the components' own attenuation is the whole
				// treatment of scattering.
				kappaEst, err := p.model.Estimate(ctx, skybrightness.Query{
					Scene: webScene, Direction: view, Grid: grid,
				})
				if err != nil {
					t.Fatalf("%s: web estimate: %v", p.id, err)
				}

				got[p.id].kappa += bandFlux(t, kappaEst.SpectralRadiance(), grid, band)

				// The full model: true extinction, plus the integral.
				directEst, err := p.model.Estimate(ctx, skybrightness.Query{
					Scene: fullScene, Direction: view, Grid: grid,
				})
				if err != nil {
					t.Fatalf("%s: direct estimate: %v", p.id, err)
				}

				got[p.id].direct += bandFlux(t, directEst.SpectralRadiance(), grid, band)

				above, err := p.model.AboveAtmosphere(skybrightness.Query{
					Scene: fullScene, Grid: grid,
				})
				if err != nil {
					t.Fatalf("%s: AboveAtmosphere: %v", p.id, err)
				}

				scattered := skybrightness.NewSpectralRadiance(grid)

				if err := skybrightness.ScatteredIn(
					ctx, scattered, above, fullScene, view, grid, rings,
				); err != nil {
					t.Fatalf("%s: ScatteredIn: %v", p.id, err)
				}

				got[p.id].scattered += bandFlux(t, scattered, grid, band)
			}
		}
	}

	star, zodi := got[skybrightness.Starlight], got[skybrightness.Zodiacal]

	t.Logf("%d epochs, %d directions in the %.0f degree zenith cap, %d quadrature rings",
		len(epochs), len(capDirs), table2CapDeg, rings)
	t.Log("")
	t.Logf("  %-10s %13s %13s %13s %13s",
		"component", "kappa model", "direct", "scattered in", "Eq. 8 total")

	for _, p := range parts {
		a := got[p.id]
		t.Logf("  %-10s %13.5e %13.5e %13.5e %13.5e",
			p.id, a.kappa, a.direct, a.scattered, a.direct+a.scattered)
	}

	if zodi.kappa <= 0 || zodi.direct+zodi.scattered <= 0 {
		t.Fatal("the zodiacal component produced no light")
	}

	var (
		kappaRatio = star.kappa / zodi.kappa
		eq11Ratio  = (star.direct + star.scattered) / (zodi.direct + zodi.scattered)
		want       = gambonsTable2["starlight"].V / gambonsTable2["zodiacal"].V
	)

	t.Log("")
	t.Logf("  starlight/zodiacal, kappa = 0.5 (web):     %.4f", kappaRatio)
	t.Logf("  starlight/zodiacal, Eq. 8 = L_d + L_s:     %.4f", eq11Ratio)
	t.Logf("  starlight/zodiacal, Table 2:               %.4f", want)
	t.Log("")

	// How much of the gap the full model closes. One means it lands exactly on
	// Table 2, zero means it moved nothing, negative means it moved away.
	gap := kappaRatio - want
	if math.Abs(gap) < 1e-9 {
		t.Skip("the simplified model already reproduces Table 2; there is no gap to close")
	}

	closed := (kappaRatio - eq11Ratio) / gap

	t.Logf("  the full scattering model closes %.0f per cent of the gap", 100*closed)

	scatterShare := func(a *totals) float64 {
		return a.scattered / (a.direct + a.scattered)
	}

	t.Logf("  scattered light is %.1f per cent of the starlight total and %.1f per cent "+
		"of the zodiacal", 100*scatterShare(star), 100*scatterShare(zodi))

	// The same quantity the composition test measures, so the two cannot drift
	// apart unnoticed. That test reports starlight and zodiacal as 34.1 and
	// 27.6 per cent of the zenith total in V; their ratio is what this test's
	// kappa column must reproduce, since it is the same model over the same
	// epochs and directions.
	const fromComposition = 34.1 / 27.6

	if rel := math.Abs(kappaRatio-fromComposition) / fromComposition; rel > 0.10 {
		t.Errorf("the kappa model gives starlight/zodiacal %.4f here and %.4f in "+
			"TestAgainstGAMBONSTable2, %.0f per cent apart; the two tests are no longer "+
			"measuring the same thing", kappaRatio, fromComposition, 100*rel)
	}

	// The scattered term must be a real but minority contribution at the
	// zenith. Masana et al. put the whole simplified-against-full difference
	// under a tenth of a magnitude in most cases, which is under ten per cent
	// in flux; a scattered share of half would mean the kernel or the
	// solid-angle weighting is wrong, and one near zero would mean the
	// integral is not running.
	for _, p := range parts {
		if s := scatterShare(got[p.id]); s < 0.01 || s > 0.40 {
			t.Errorf("%s: scattered light is %.1f per cent of the total, which is outside "+
				"anything a clear atmosphere at these optical depths can produce",
				p.id, 100*s)
		}
	}

	// Both models must still describe the same sky to within the tenth of a
	// magnitude the paper allows between them.
	if rel := math.Abs(eq11Ratio-kappaRatio) / kappaRatio; rel > 0.25 {
		t.Errorf("the two models disagree by %.0f per cent on the ratio; the paper puts the "+
			"whole difference between them under a tenth of a magnitude", 100*rel)
	}
}
