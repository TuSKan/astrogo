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

// Why our zenith starlight-to-zodiacal ratio can differ from Table 2's without
// either star map or zodiacal model being wrong.
//
// # The argument
//
// Under the effective-optical-depth transfer this module implements, every
// extended component in a given direction is multiplied by the same
// exp(-tau_eff * m): same airmass, same optical depth, same factor. A ratio
// between two extended components at one direction is therefore INVARIANT
// under any change to kappa, to the airmass law or to the aerosol load. This is
// not a claim about the implementation, it is arithmetic — and it is confirmed
// by TestPresetsDifferOnlyInTransfer, where changing kappa moves both presets'
// components together.
//
// Masana et al. (2024) Table 2 comes from their full model, Eq. 11, which adds
// a term the simplified transfer has no counterpart for: light scattered INTO
// the beam from every other direction of the sky. That term is proportional to
// the component's radiance over the whole hemisphere, not to its radiance in
// the direction being observed. It is therefore NOT a common factor: a
// component with much of its light near the horizon gains more of it at the
// zenith than a component concentrated overhead.
//
// So a discrepancy in a ratio of two extended components at the zenith is the
// expected signature of the missing scattered-in term, and cannot be evidence
// against either component on its own. This test measures whether the effect
// has the size and the sign to account for what is observed.
//
// # The prediction, and what came of it
//
// Our starlight-to-zodiacal ratio is measured HIGHER than Table 2's. For the
// scattered-in term to explain that, it must boost zodiacal at the zenith more
// than it boosts starlight, which requires zodiacal light to have the larger
// hemisphere-mean-to-zenith ratio.
//
// It does: measured, zodiacal comes out near 1.05 and starlight near 0.87, so
// the sign is right. The size is not. Closing the whole gap this way would
// need a scattered-in coefficient of about 2.7, an order of magnitude more
// than the roughly 0.22 of scattering optical depth the scene has to do it
// with. At the depth actually available the term accounts for about a quarter
// of the difference.
//
// So this rules the mechanism IN as a contributor and OUT as the explanation,
// and the remaining three quarters is a genuine open question rather than
// something already accounted for. It is recorded as such in
// docs/skybrightness.md, and it is the reason Table 2 cannot be used to
// attribute a discrepancy to the star map: a component-by-component
// comparison against a full-model composition is confounded by a term this
// module does not implement, in an amount that is small but not negligible.

// bandCentreNM is the effective wavelength of Johnson-Cousins V as Table 1 of
// Masana et al. (2024) gives it, and the wavelength the scattering optical
// depth is evaluated at.
const bandCentreNM unit.WavelengthNM = 552.4

func TestScatteredInTermExplainsTheZenithRatio(t *testing.T) {
	testutil.RequireReachable(t, "github.com:443")

	ctx, cancel := context.WithTimeout(context.Background(), 30*gotime.Minute)
	defer cancel()

	remote.EnableDownloads(32<<20, remote.GaiaStarMap)

	grid := skybrightness.DefaultOpticalGrid()
	band := johnsonVFromTable1()

	site, err := coord.NewGeodetic(angle.Deg(0), angle.Deg(table2LatDeg), table2ElevM)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	atm, err := atmosphere.NewBuilder().
		Surface(1013, 288).
		Aerosol(table2AOD550, 550, table2Angstrom, table2AerosolW, table2Asym).
		BoundaryLayer(1000).
		Build()
	if err != nil {
		t.Fatalf("atmosphere Build: %v", err)
	}

	provider := eph.Default()

	epochs := table2Epochs(t, site, provider, atm)
	if len(epochs) == 0 {
		t.Fatal("no astronomical-night epochs were found")
	}

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

	// Starlight and zodiacal light alone. Neither needs a dust map or a
	// SkyCalc spectrum, so this runs off the star map and the ephemeris and
	// asks nothing of IRSA or ESO.
	model, err := skybrightness.NewModel("scattered-in-probe",
		isl, skybrightness.NewZodiacalLight())
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	// A solid-angle-weighted hemisphere. dOmega = cos(alt) dalt daz, so each
	// ring carries the cosine of its altitude.
	const (
		altStepDeg = 5.0
		azStepDeg  = 30.0
	)

	type accum struct{ zenith, hemi, weight float64 }

	got := map[skybrightness.ComponentID]*accum{
		skybrightness.Starlight: {},
		skybrightness.Zodiacal:  {},
	}

	fluxes := func(when gotime.Time, d coord.AltAz) (map[skybrightness.ComponentID]float64, bool) {
		est, err := model.Estimate(ctx, skybrightness.Query{
			Scene: &skybrightness.Scene{
				Observer: site, Time: when, Atmosphere: atm, Ephemeris: provider,
			},
			Direction: d,
			Grid:      grid,
		})
		if err != nil {
			// Zodiacal light is undefined close to the Sun. Dropping the
			// direction for BOTH components keeps the two means over the same
			// set of sightlines, which is the only way the ratio of the means
			// stays meaningful.
			return nil, false
		}

		out := make(map[skybrightness.ComponentID]float64, len(got))

		for id := range got {
			spectrum, ok := est.Component(id)
			if !ok {
				return nil, false
			}

			out[id] = bandFlux(t, spectrum, grid, band)
		}

		return out, true
	}

	var skipped, used int

	for _, when := range epochs {
		zen, ok := fluxes(when, coord.NewAltAz(angle.Deg(90), angle.Deg(0)))
		if !ok {
			continue
		}

		for id, f := range zen {
			got[id].zenith += f
		}

		for altDeg := altStepDeg / 2; altDeg < 90; altDeg += altStepDeg {
			w := math.Cos(angle.Deg(altDeg).Radians())

			for azDeg := 0.0; azDeg < 360; azDeg += azStepDeg {
				f, ok := fluxes(when, coord.NewAltAz(angle.Deg(altDeg), angle.Deg(azDeg)))
				if !ok {
					skipped++

					continue
				}

				used++

				for id, v := range f {
					got[id].hemi += v * w
					got[id].weight += w
				}
			}
		}
	}

	if used == 0 {
		t.Fatal("no usable sightlines")
	}

	t.Logf("%d epochs, %d hemisphere sightlines used, %d dropped near the Sun",
		len(epochs), used, skipped)
	t.Log("")
	t.Logf("  %-12s %14s %14s %10s", "component", "zenith", "hemi mean", "mean/zenith")

	ratio := map[skybrightness.ComponentID]float64{}

	for _, id := range []skybrightness.ComponentID{
		skybrightness.Starlight, skybrightness.Zodiacal,
	} {
		a := got[id]
		zenith := a.zenith / float64(len(epochs))
		hemi := a.hemi / a.weight
		ratio[id] = hemi / zenith

		t.Logf("  %-12s %14.6e %14.6e %10.4f", id, zenith, hemi, ratio[id])
	}

	rStar, rZodi := ratio[skybrightness.Starlight], ratio[skybrightness.Zodiacal]

	t.Log("")

	// The prediction. If this fails, the scattered-in term is not the
	// explanation and the discrepancy has to be sought in the components.
	if rZodi <= rStar {
		t.Errorf("zodiacal mean/zenith is %.4f and starlight's is %.4f; the scattered-in term "+
			"would then boost starlight at the zenith more than zodiacal, which is the opposite "+
			"of the sign needed to explain our ratio being the higher one — so the missing "+
			"scattered-in term does NOT account for the Table 2 difference",
			rZodi, rStar)
	}

	// How much of the gap it actually closes, at the optical depth this scene
	// actually has.
	//
	// To first order the zenith radiance of component c gains a term
	// proportional to its hemisphere mean, L_zen -> L_zen * (1 + tau_s * R_c),
	// with tau_s the scattering optical depth — the same for every component,
	// since it is a property of the air and not of the source. Only the
	// anisotropy differs, which is why R_c is what carries the effect. (For a
	// perfectly isotropic field R_c is 1 for everyone, scattering in and
	// scattering out cancel, and the ratio is untouched — the classical
	// result, and the reason the effect here is small.)
	pressure, _ := atm.Surface()

	rayleigh, err := atmosphere.RayleighOpticalDepth(bandCentreNM, float64(pressure))
	if err != nil {
		t.Fatalf("RayleighOpticalDepth: %v", err)
	}

	aer := atm.Aerosol()
	tauScat := float64(rayleigh) +
		float64(aer.TauAt(bandCentreNM))*float64(aer.SingleScatteringAlbedo)

	achieved := (1 + tauScat*rStar) / (1 + tauScat*rZodi)

	// What the ratio would have to be multiplied by to land on Table 2.
	const (
		oursStarOverZodi   = 34.1 / 27.6 // measured by TestAgainstGAMBONSTable2
		theirsStarOverZodi = 27.2 / 25.3 // Table 2
	)

	target := theirsStarOverZodi / oursStarOverZodi

	explained := (1 - achieved) / (1 - target)

	t.Logf("scattering optical depth at %.1f nm: %.4f (Rayleigh %.4f, aerosol scattering %.4f)",
		float64(bandCentreNM), tauScat, float64(rayleigh),
		float64(aer.TauAt(bandCentreNM))*float64(aer.SingleScatteringAlbedo))
	t.Logf("the gap to close is a factor %.4f; the scattered-in term supplies %.4f",
		target, achieved)
	t.Logf("so it accounts for %.0f per cent of the Table 2 difference, and %.0f per cent is "+
		"NOT explained by it", 100*explained, 100*(1-explained))

	// The finding, recorded as a bound rather than a point so that it is a
	// test and not a transcription.
	//
	// The lower bound fails if the mechanism disappears — if the two fields
	// ever become equally anisotropic, this stops being an explanation for any
	// of the gap and the whole difference moves to the components. The upper
	// bound fails if it ever explains nearly all of it, which would mean
	// something else changed and the remaining discrepancy documented in
	// docs/skybrightness.md is no longer real.
	const (
		minExplained = 0.10
		maxExplained = 0.60
	)

	if explained < minExplained || explained > maxExplained {
		t.Errorf("the scattered-in term explains %.0f per cent of the Table 2 starlight-to-"+
			"zodiacal difference, outside the %.0f to %.0f per cent this was measured at; "+
			"the balance between it and whatever else drives the difference has moved",
			100*explained, 100*minExplained, 100*maxExplained)
	}
}
