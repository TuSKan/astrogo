package optics_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/optics"
	"github.com/TuSKan/astrogo/unit"
)

// The energy-to-photon conversion must happen inside the integral, at each
// sample's own wavelength.
//
// TestPhotonRateAgainstClosedForm already reproduces the quadrature exactly,
// but it cannot catch this particular mistake: it uses a flat spectrum on a
// symmetric grid, and the photon conversion 1/E = lambda/hc is linear in
// lambda, so its mean across that grid equals its value at the centre.
// Converting once outside the integral, at a mean or pivot wavelength, gives
// the same answer there — and a different, wrong one for every spectrum that
// is not flat.
//
// What separates the two is a spectrum with structure. Equal energy heaped at
// the red end and at the blue end is not equal numbers of photons: a red
// photon carries less energy, so the same joules are more of them, by the
// ratio of the wavelengths. Done outside the integral, the two would come out
// identical.
func TestPhotonRateWeightsByWavelengthNotEnergy(t *testing.T) {
	t.Parallel()

	grid, err := unit.NewSpectralGrid(400, 1, 401) // 400..800 nm
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	inst := optics.Instrument{
		Name:              "flat response",
		CollectingAreaM2:  1,
		PixelSolidAngleSR: 1,
		Throughput: []optics.Throughput{{
			Name:         "wide",
			WavelengthNM: []unit.WavelengthNM{399, 400, 800, 801},
			Efficiency:   []float64{0, 1, 1, 0},
			Reference:    "synthetic test fixture, not a measured curve",
		}},
	}

	// Two narrow blocks of identical width and identical spectral radiance,
	// so identical energy, one centred at 440 nm and one at 760 nm.
	blue := make([]float64, grid.Len())
	red := make([]float64, grid.Len())

	for i := range grid.Len() {
		nm := float64(grid.At(i))

		switch {
		case nm >= 420 && nm <= 460:
			blue[i] = 1e-6
		case nm >= 740 && nm <= 780:
			red[i] = 1e-6
		}
	}

	blueRate, err := inst.PhotonRate(blue, grid)
	if err != nil {
		t.Fatalf("blue: %v", err)
	}

	redRate, err := inst.PhotonRate(red, grid)
	if err != nil {
		t.Fatalf("red: %v", err)
	}

	ratio := float64(redRate) / float64(blueRate)
	want := 760.0 / 440.0

	if math.Abs(ratio-want)/want > 0.01 {
		t.Errorf("equal energy in the red and in the blue gave a photon ratio of %.5f, want about "+
			"%.5f, the ratio of the wavelengths. A ratio of 1 would mean the energy-to-photon "+
			"conversion happens outside the integral", ratio, want)
	}

	// And the direction, stated on its own: the red block is the more photons.
	if redRate <= blueRate {
		t.Errorf("the red block gave %.6g photons against the blue block's %.6g; the same energy "+
			"at a longer wavelength is always more photons", float64(redRate), float64(blueRate))
	}
}
