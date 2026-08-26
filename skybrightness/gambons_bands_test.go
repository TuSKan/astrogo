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
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/skybrightness/dataset/airglow"
	"github.com/TuSKan/astrogo/skybrightness/dataset/passband"
)

// Table 2 across all five Johnson-Cousins bands, on the one ratio that can be
// formed without a star map.
//
// # Why a ratio and not the composition
//
// Table 2 gives each component as a percentage of the total in U, B, V, R and
// I. Reproducing the whole composition in a band needs every component in that
// band, and two of them cannot be had: integrated starlight comes from a
// tabulated map that exists only in V, and diffuse galactic light is
// correlated against that same map, so it is V-only for the same reason.
// Building the other four maps means re-running the Gaia aggregation four
// times, which is a data-preparation job and not something a test does.
//
// Zodiacal light and airglow need neither. Their ratio is dimensionless, it is
// immune to the zero point and to the absolute airglow normalisation that
// dominated every earlier comparison, and it is available in all five bands
// from the paper's own table. What it tests is the thing the V comparison
// found worst: the airglow spectrum, whose shape across the optical range is
// exactly what a five-band ratio is sensitive to and a single band is not.
//
// # What the bands are
//
// Real Bessell (1990) response curves fetched from SVO, with the detector
// convention and zero point the service publishes, rather than a tophat. The
// difference matters most here: the airglow spectrum is a forest of lines on a
// continuum, so how much of it a band admits depends on the wings of the real
// curve and not on where a tophat's edges are put. The OI line at 558 nm sits
// inside V, and the OH bands dominate I.
func TestAgainstGAMBONSTable2AcrossBands(t *testing.T) {
	testutil.RequireReachable(t, "svo2.cab.inta-csic.es:443")
	testutil.RequireReachable(t, "etimecalret-002.eso.org:443")

	ctx, cancel := context.WithTimeout(context.Background(), 40*gotime.Minute)
	defer cancel()

	grid := skybrightness.DefaultOpticalGrid()

	site, err := coord.NewGeodetic(angle.Deg(0), angle.Deg(table2LatDeg), table2ElevM)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	atm, err := atmosphere.NewBuilder().
		Surface(1013, 288).
		Aerosol(table2AOD550, 550, table2Angstrom, table2AerosolW, table2Asym).
		AerosolScaleHeight(1000).
		Build()
	if err != nil {
		t.Fatalf("atmosphere Build: %v", err)
	}

	provider := eph.Default()

	epochs := table2Epochs(t, site, provider, atm)
	if len(epochs) == 0 {
		t.Fatal("no astronomical-night epochs were found")
	}

	capDirs := zenithCap(table2CapDeg, table2CapSamples)

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

	model, err := skybrightness.NewModel("gambons-bands",
		skybrightness.NewZodiacalLight(), glow)
	if err != nil {
		t.Fatalf("NewModel: %v", err)
	}

	// The five bands, and the ratio Table 2 implies for each.
	bands := []struct {
		key    string
		filter string
		want   float64
	}{
		{"U", "Generic/Bessell.U", gambonsTable2["zodiacal"].U / gambonsTable2["airglow"].U},
		{"B", "Generic/Bessell.B", gambonsTable2["zodiacal"].B / gambonsTable2["airglow"].B},
		{"V", "Generic/Bessell.V", gambonsTable2["zodiacal"].V / gambonsTable2["airglow"].V},
		{"R", "Generic/Bessell.R", gambonsTable2["zodiacal"].R / gambonsTable2["airglow"].R},
		{"I", "Generic/Bessell.I", gambonsTable2["zodiacal"].I / gambonsTable2["airglow"].I},
	}

	t.Log("")
	t.Log("zenith zodiacal-to-airglow ratio, Johnson-Cousins, real Bessell curves:")
	t.Logf("  %-3s %10s %10s %10s %9s", "band", "astrogo", "Table 2", "ours/theirs", "coverage")

	type result struct {
		key      string
		got      float64
		want     float64
		coverage float64
	}

	var results []result

	for _, b := range bands {
		band, err := passband.Fetch(ctx, b.filter)
		if err != nil {
			t.Errorf("%s: %v", b.filter, err)

			continue
		}

		// How much of the band the grid actually spans. U runs blueward of
		// where DefaultOpticalGrid starts, so its answer is over a truncated
		// band and has to be reported rather than quietly used.
		_, coverage, err := band.Weights(grid)
		if err != nil {
			t.Errorf("%s: Weights: %v", b.filter, err)

			continue
		}

		var zodi, glowFlux float64

		for _, when := range epochs {
			scene := &skybrightness.Scene{
				Observer: site, Time: when, Atmosphere: atm, Ephemeris: provider,
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
					case skybrightness.Zodiacal:
						zodi += flux
					case skybrightness.AirglowContinuum, skybrightness.AirglowLines:
						glowFlux += flux
					case skybrightness.Starlight, skybrightness.DiffuseGalactic,
						skybrightness.Extragalactic, skybrightness.Moonlight,
						skybrightness.Twilight, skybrightness.Artificial:
						t.Fatalf("component %q is registered but this model has only "+
							"zodiacal light and airglow", id)
					}
				}
			}
		}

		if glowFlux <= 0 {
			t.Errorf("%s: the airglow contributed nothing", b.key)

			continue
		}

		got := zodi / glowFlux

		t.Logf("  %-3s %10.4f %10.4f %10.3f %8.0f%%",
			b.key, got, b.want, got/b.want, 100*coverage)

		results = append(results, result{b.key, got, b.want, coverage})
	}

	if len(results) < 4 {
		t.Fatalf("only %d bands resolved; there is nothing to compare across", len(results))
	}

	// The shape of the disagreement across the spectrum, which is what five
	// bands buy over one.
	//
	// A ratio that is off by the same factor in every band is a normalisation:
	// one of the two components is scaled wrong and the airglow spectrum's
	// shape is right. A ratio that drifts from U to I is a colour error — the
	// airglow spectrum has the wrong shape, or the zodiacal colour correction
	// does. The two call for entirely different work, and a single band cannot
	// tell them apart.
	lo, hi := math.Inf(1), math.Inf(-1)

	var logSum float64

	for _, r := range results {
		rel := r.got / r.want
		lo, hi = math.Min(lo, rel), math.Max(hi, rel)
		logSum += math.Log(rel)
	}

	mean := math.Exp(logSum / float64(len(results)))
	spread := hi / lo

	t.Log("")
	t.Logf("across %d bands: mean ours/theirs %.3f, spread %.2fx (worst %.3f to %.3f)",
		len(results), mean, spread, lo, hi)

	switch {
	case spread < 1.3:
		t.Logf("the disagreement is flat across the spectrum, so it is a normalisation " +
			"rather than a colour error: the airglow spectrum's SHAPE agrees with GAMBONS " +
			"and its LEVEL does not")
	default:
		t.Logf("the disagreement varies by %.2fx across the spectrum, so it is a colour "+
			"error and not only a normalisation: the airglow spectrum's shape differs from "+
			"GAMBONS', or the zodiacal colour correction does", spread)
	}

	// Bounds, not a transcription. These are wide because this is the first
	// five-band measurement and its purpose is to characterise the
	// disagreement rather than to pin a number that has not been explained.
	// They fail if the model stops resembling GAMBONS at all in any band,
	// which is what would happen if a spectrum were replaced or a band
	// misidentified.
	for _, r := range results {
		if rel := r.got / r.want; rel < 0.25 || rel > 4 {
			t.Errorf("%s: zodiacal/airglow is %.4f against Table 2's %.4f, a factor %.2f — "+
				"that is not a calibration difference, it is a different sky",
				r.key, r.got, r.want, rel)
		}
	}
}
