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
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/skybrightness/dataset/airglow"
	"github.com/TuSKan/astrogo/skybrightness/dataset/dust"
	"github.com/TuSKan/astrogo/skybrightness/dataset/starlight"
	astrotime "github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// Table 2 of Masana, Bará, Carrasco & Ribas (2024), "An enhanced version of
// the Gaia map of the brightness of the natural sky", International Journal of
// Sustainable Lighting, arXiv:2408.17371: the percentage each component
// contributes to the zenith natural night sky brightness, per Johnson-Cousins
// band.
//
// This is a better target than the web tool's all-sky export, for three
// reasons. It is a composition, so it is dimensionless and immune to zero
// points, to passband normalisation and to the airglow level — the very things
// that dominated the all-sky comparison. It comes from the paper's full
// scattering model rather than the web version's simplified one, whose authors
// state it runs bright near the horizon and dark at the zenith. And every
// parameter behind it is published.
var gambonsTable2 = map[string]struct{ U, B, V, R, I float64 }{
	"airglow":    {64.6, 35.7, 44.0, 55.9, 77.0},
	"starlight":  {20.8, 34.3, 27.2, 21.7, 12.4},
	"zodiacal":   {12.4, 26.3, 25.3, 19.6, 9.0},
	"background": {2.2, 3.7, 3.5, 2.8, 1.6},
}

// The scene Table 2 was computed for, from the paper's own description.
const (
	table2LatDeg   = 40.0
	table2ElevM    = 0
	table2AOD550   = 0.15
	table2Angstrom = 1.0
	table2AerosolW = 0.85 // aerosol single-scattering albedo
	table2Asym     = 0.9  // scattering asymmetry parameter
	table2SolarSFU = 100
	table2CapDeg   = 10.0 // "a region of the sky of radius 10 degrees around the zenith"
)

// The sampling this test can afford, against the paper's hourly-for-a-year.
const (
	table2Months        = 12
	table2HoursPerNight = 8
	table2CapSamples    = 4
)

// johnsonVFromTable1 is a tophat over Johnson-Cousins V as Table 1 of the same
// paper characterises it: effective wavelength 552.4 nm, width 90.9 nm.
//
// Still a tophat rather than the real curve, but centred and sized on their
// own numbers rather than on a round 500 to 600, which matters more here than
// usual because the comparison is between bands whose airglow content differs
// sharply across exactly this range — the OI line at 558 nm sits inside it.
func johnsonVFromTable1() magnitude.Passband {
	const (
		centre = 552.4
		width  = 90.9
	)

	lo, hi := centre-width/2, centre+width/2

	return magnitude.Passband{
		Name: "Johnson-Cousins V (tophat on Masana et al. 2024 Table 1)",
		WavelengthNM: []unit.WavelengthNM{
			unit.WavelengthNM(lo - 1), unit.WavelengthNM(lo),
			unit.WavelengthNM(hi), unit.WavelengthNM(hi + 1),
		},
		Response: []float64{0, 1, 1, 0},

		// Energy, not photons. Masana et al. (2021) Section 3.2 state they take
		// Bessell & Murphy's photonic V and transform it back to its original
		// energy response form, because the classical V system is defined on
		// in-band irradiance rather than photon number.
		Detector:        magnitude.EnergyIntegrating,
		VegaZeroPointJy: 3636,
		Reference:       "Masana et al. (2024) Table 1, effective wavelength and width",
	}
}

// The composition of the zenith sky, against Table 2.
//
// The paper averages the zenith brightness hourly from the end to the
// beginning of astronomical twilight, over a full year. This samples twelve
// months and four hours a night, which is coarser but covers the same thing
// the average is for: over a year the zenith at a fixed latitude sweeps the
// whole circle of declination 40, so the Milky Way and the ecliptic pass
// through it and out again, and a single night says nothing about the mean.
func TestAgainstGAMBONSTable2(t *testing.T) {
	testutil.RequireReachable(t, "irsa.ipac.caltech.edu:443")
	testutil.RequireReachable(t, "etimecalret-002.eso.org:443")
	testutil.RequireReachable(t, "github.com:443")

	ctx, cancel := context.WithTimeout(context.Background(), 60*gotime.Minute)
	defer cancel()

	enableStarMapDownload(t)

	grid := skybrightness.DefaultOpticalGrid()
	band := johnsonVFromTable1()

	site, err := coord.NewGeodetic(angle.Deg(0), angle.Deg(table2LatDeg), table2ElevM)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	// The atmosphere the paper states: aerosol optical depth 0.15 at an
	// Angstrom exponent of 1, aerosol albedo 0.85, asymmetry 0.9.
	atm, err := atmosphere.NewBuilder().
		Surface(1013, 288).
		Aerosol(table2AOD550, 550, table2Angstrom, table2AerosolW, table2Asym).
		AerosolScaleHeight(1000).
		Build()
	if err != nil {
		t.Fatalf("atmosphere Build: %v", err)
	}

	provider := eph.Default()

	// Epochs: twelve months, and within each night the hours when the Sun is
	// below eighteen degrees, which is the window the paper averages over.
	epochs := table2Epochs(t, site, provider, atm)
	if len(epochs) == 0 {
		t.Fatal("no astronomical-night epochs were found")
	}

	t.Logf("sampling %d epochs across the year, %d directions in a %.0f degree zenith cap",
		len(epochs), table2CapSamples, table2CapDeg)

	// Every sightline this test will look along, so dust is fetched once.
	capDirs := zenithCap(table2CapDeg, table2CapSamples)

	dirs := make([]dust.Direction, 0, len(capDirs)*len(epochs))

	for _, when := range epochs {
		cc := coord.NewContext(astrotime.FromGo(when), site, atm.Refraction())

		for _, d := range capDirs {
			icrs, err := cc.AltAzToICRS(d)
			if err != nil {
				t.Fatalf("AltAzToICRS: %v", err)
			}

			gal := coord.ICRSToGalactic(icrs)
			dirs = append(dirs, dust.Direction{L: gal.L(), B: gal.B()})
		}
	}

	dustMap := dust.NewMap()

	for start := 0; start < len(dirs); start += dustChunk {
		end := min(start+dustChunk, len(dirs))

		if _, err := dust.Fetch(ctx, dustMap, dirs[start:end]...); err != nil {
			t.Skipf("IRSA stopped answering after %d of %d sightlines: %v", dustMap.Len(), len(dirs), err)
		}
	}

	t.Logf("dust: %d distinct cells for %d sightlines", dustMap.Len(), len(dirs))

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

	dgl, err := skybrightness.NewDiffuseGalacticLight(dustMap, stars, band)
	if err != nil {
		t.Fatalf("NewDiffuseGalacticLight: %v", err)
	}

	glow, err := airglow.NewAirglow(ctx, airglow.Spec{
		Observatory:  airglow.Paranal,
		SolarFluxSFU: table2SolarSFU,
		MinNM:        float64(grid.At(0)) - 1,
		MaxNM:        float64(grid.At(grid.Len()-1)) + 1,
		StepNM:       0.1,
	}, grid, 87_000)
	if err != nil {
		t.Skipf("SkyCalc did not answer: %v", err)
	}

	model, err := skybrightness.NewModel("gambons-table2",
		isl, dgl, skybrightness.NewZodiacalLight(), glow,
		skybrightness.NewExtragalacticBackground())
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	// Accumulate each component's band radiance over every epoch and every cap
	// direction. Percentages of a mean radiance, formed in radiance.
	share := map[string]float64{}

	var total float64

	for _, when := range epochs {
		scene := &skybrightness.Scene{
			Observer:   site,
			Time:       when,
			Atmosphere: atm,
			Ephemeris:  provider,
		}

		for _, d := range capDirs {
			est, err := model.Estimate(ctx,
				skybrightness.Query{Scene: scene, Direction: d, Grid: grid})
			if err != nil {
				t.Fatalf("Estimate at %v: %v", when, err)
			}

			for _, id := range est.ComponentIDs() {
				spectrum, ok := est.Component(id)
				if !ok {
					continue
				}

				flux := bandFlux(t, spectrum, grid, band)

				switch id {
				case skybrightness.AirglowContinuum, skybrightness.AirglowLines:
					share["airglow"] += flux
				case skybrightness.Starlight:
					share["starlight"] += flux
				case skybrightness.Zodiacal:
					share["zodiacal"] += flux
				case skybrightness.DiffuseGalactic, skybrightness.Extragalactic:
					share["background"] += flux

				case skybrightness.Moonlight, skybrightness.Twilight, skybrightness.Artificial:
					// GAMBONS is a model of the natural sky on a moonless
					// night, so Table 2 has no column for any of these and a
					// scene that produced one would not be comparable with it.
					t.Fatalf("component %q has no counterpart in Table 2; this scene is not "+
						"the one the table was computed for", id)
				}

				total += flux
			}
		}
	}

	if total <= 0 {
		t.Fatal("the model produced no light at all")
	}

	t.Log("")
	t.Log("zenith composition in Johnson-Cousins V, per cent of band radiance:")
	t.Logf("  %-22s %9s %9s %9s", "component", "astrogo", "Table 2", "diff")

	var worst float64

	var worstName string

	for _, name := range []string{"airglow", "starlight", "zodiacal", "background"} {
		got := 100 * share[name] / total
		want := gambonsTable2[name].V
		diff := got - want

		if math.Abs(diff) > math.Abs(worst) {
			worst, worstName = diff, name
		}

		t.Logf("  %-22s %8.1f%% %8.1f%% %+8.1f", name, got, want, diff)
	}

	t.Logf("  worst component: %s, %+.1f percentage points", worstName, worst)

	// A composition has to be one, whatever else is true of it.
	var sum float64
	for _, name := range []string{"airglow", "starlight", "zodiacal", "background"} {
		sum += 100 * share[name] / total
	}

	if math.Abs(sum-100) > 0.01 {
		t.Errorf("the components account for %.3f per cent of the radiance, not 100", sum)
	}

	// The ordering is the qualitative claim, and it should survive sampling
	// this much coarser than the paper's.
	if share["airglow"] <= share["starlight"] || share["starlight"] <= share["background"] {
		t.Errorf("in V the order should be airglow, then starlight and zodiacal, then background; got %v", share)
	}
}

// table2Epochs returns sample times spread across the year, restricted to when
// the Sun is below eighteen degrees.
func table2Epochs(
	t *testing.T,
	site *coord.Geodetic,
	provider eph.Provider,
	atm *atmosphere.Atmosphere,
) []gotime.Time {
	t.Helper()

	var out []gotime.Time

	for month := range table2Months {
		// The fifteenth of each month, which is close enough to the middle for
		// a yearly mean of something that varies smoothly with solar longitude.
		day := gotime.Date(2026, gotime.Month(month+1), 15, 0, 0, 0, 0, gotime.UTC)

		var found int

		// Walk the night in half-hour steps and keep the hours that are dark,
		// spread across the window rather than clustered at its start.
		var dark []gotime.Time

		for step := range 48 {
			when := day.Add(gotime.Duration(step*30) * gotime.Minute) //nolint:durationcheck // step*30 is a count of minutes, not a duration

			cc := coord.NewContext(astrotime.FromGo(when), site, atm.Refraction())

			sun, err := eph.Position(provider, eph.Sun, astrotime.FromGo(when))
			if err != nil {
				continue
			}

			if cc.GeocentricToObserved(sun).Alt().Degrees() < -18 {
				dark = append(dark, when)
			}
		}

		if len(dark) == 0 {
			continue
		}

		for k := range table2HoursPerNight {
			idx := (2*k + 1) * len(dark) / (2 * table2HoursPerNight)
			out = append(out, dark[idx])

			found++
		}
	}

	return out
}
