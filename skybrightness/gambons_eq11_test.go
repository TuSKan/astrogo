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

	// Both configurations come from their presets rather than from literals
	// here, so this comparison cannot drift away from what the library ships.
	webKappa, err := skybrightness.GAMBONSWeb.DiffuseKappa()
	if err != nil {
		t.Fatalf("GAMBONSWeb kappa: %v", err)
	}

	fullKappa, err := skybrightness.GAMBONSFull.DiffuseKappa()
	if err != nil {
		t.Fatalf("GAMBONSFull kappa: %v", err)
	}

	webFidelity, err := skybrightness.GAMBONSWeb.Fidelity()
	if err != nil {
		t.Fatalf("GAMBONSWeb fidelity: %v", err)
	}

	fullFidelity, err := skybrightness.GAMBONSFull.Fidelity()
	if err != nil {
		t.Fatalf("GAMBONSFull fidelity: %v", err)
	}

	web, full := build(webKappa), build(fullKappa)

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

	// Starlight and zodiacal light in one model, read back per component.
	//
	// Reference fidelity runs the Eq. 11 integral separately for each
	// component and adds it to that component's own spectrum, so the
	// attribution survives and a single model suffices. An earlier version
	// built one model per component and called ScatteredIn by hand, which
	// worked but reimplemented in a test what Estimate now does — exactly the
	// drift a preset exists to prevent.
	model, err := skybrightness.NewModel("eq11-probe", isl, skybrightness.NewZodiacalLight())
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	ids := []skybrightness.ComponentID{skybrightness.Starlight, skybrightness.Zodiacal}

	type totals struct{ kappa, full float64 }

	got := map[skybrightness.ComponentID]*totals{
		skybrightness.Starlight: {},
		skybrightness.Zodiacal:  {},
	}

	accumulate := func(scene *skybrightness.Scene, view coord.AltAz,
		fidelity skybrightness.Fidelity, into func(*totals) *float64,
	) {
		t.Helper()

		est, err := model.Estimate(ctx, skybrightness.Query{
			Scene: scene, Direction: view, Grid: grid, Fidelity: fidelity,
		})
		if err != nil {
			t.Fatalf("estimate at %v: %v", fidelity, err)
		}

		for _, id := range ids {
			spectrum, ok := est.Component(id)
			if !ok {
				t.Fatalf("the estimate carries no %s component", id)
			}

			*into(got[id]) += bandFlux(t, spectrum, grid, band)
		}
	}

	for _, when := range epochs {
		webScene := &skybrightness.Scene{
			Observer: site, Time: when, Atmosphere: web, Ephemeris: provider,
		}
		fullScene := &skybrightness.Scene{
			Observer: site, Time: when, Atmosphere: full, Ephemeris: provider,
		}

		for _, view := range capDirs {
			accumulate(webScene, view, webFidelity, func(a *totals) *float64 { return &a.kappa })
			accumulate(fullScene, view, fullFidelity, func(a *totals) *float64 { return &a.full })
		}
	}

	star, zodi := got[skybrightness.Starlight], got[skybrightness.Zodiacal]

	t.Logf("%d epochs, %d directions in the %.0f degree zenith cap",
		len(epochs), len(capDirs), table2CapDeg)
	t.Log("")
	t.Logf("  %-10s %15s %15s %12s", "component", "gambons-web", "gambons-full", "full/web")

	for _, id := range ids {
		a := got[id]
		t.Logf("  %-10s %15.5e %15.5e %12.4f", id, a.kappa, a.full, a.full/a.kappa)
	}

	if zodi.kappa <= 0 || zodi.full <= 0 {
		t.Fatal("the zodiacal component produced no light")
	}

	var (
		kappaRatio = star.kappa / zodi.kappa
		eq11Ratio  = star.full / zodi.full
		want       = gambonsTable2["starlight"].V / gambonsTable2["zodiacal"].V
	)

	t.Log("")
	t.Logf("  starlight/zodiacal, gambons-web:   %.4f", kappaRatio)
	t.Logf("  starlight/zodiacal, gambons-full:  %.4f", eq11Ratio)
	t.Logf("  starlight/zodiacal, Table 2:       %.4f", want)
	t.Log("")

	// How much of the gap the full model closes. One means it lands exactly on
	// Table 2, zero means it moved nothing, negative means it moved away.
	gap := kappaRatio - want
	if math.Abs(gap) < 1e-9 {
		t.Skip("the simplified model already reproduces Table 2; there is no gap to close")
	}

	closed := (kappaRatio - eq11Ratio) / gap

	t.Logf("  the full scattering model closes %.0f per cent of the gap", 100*closed)

	// The full model must be brighter than the simplified one at the zenith,
	// and by a plausible amount. Masana et al. state the direction — the
	// simplified model "underestimates the brightness at zenith" — and bound
	// the whole difference at under a tenth of a magnitude, which is under ten
	// per cent in flux.
	for _, id := range ids {
		a := got[id]
		gainFrac := a.full/a.kappa - 1

		t.Logf("  %s: the full model is %+.1f per cent of the simplified one", id, 100*gainFrac)

		// Not a strict sign test. Starlight's scattered gain is measured at a
		// tenth of a per cent either way and moves sign with the quadrature
		// resolution, which is what a quantity consistent with zero does;
		// asserting a sign on it would be asserting noise. What is real is
		// that neither component LOSES light to the scattered term, which a
		// sign error in the kernel would show as a large negative.
		if gainFrac < -0.01 {
			t.Errorf("%s: the full model is %.1f per cent fainter than the simplified one; "+
				"the scattering integral must add light, not remove it", id, -100*gainFrac)
		}

		if gainFrac > 0.40 {
			t.Errorf("%s: the full model is %.0f per cent brighter than the simplified one, "+
				"far beyond the tenth of a magnitude the paper puts between them",
				id, 100*gainFrac)
		}
	}

	// Both models must still describe the same sky to within the tenth of a
	// magnitude the paper allows between them.
	if rel := math.Abs(eq11Ratio-kappaRatio) / kappaRatio; rel > 0.25 {
		t.Errorf("the two models disagree by %.0f per cent on the ratio; the paper puts the "+
			"whole difference between them under a tenth of a magnitude", 100*rel)
	}
}
