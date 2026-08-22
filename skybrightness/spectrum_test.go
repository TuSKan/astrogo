package skybrightness_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/unit"
)

// The blackbody shape peaks where Wien's law says it should.
//
// An independent check on the whole expression rather than on its pieces: Wien
// displacement, lambda_max * T = 2.897771955e-3 m K, comes from differentiating
// Planck's law and appears nowhere in the implementation, so a mistyped
// constant or a wrong power of lambda moves the peak off it.
func TestBlackbodyShapePeaksAtWien(t *testing.T) {
	t.Parallel()

	// A 1 nm grid wide enough to hold the peak for every temperature below.
	grid, err := unit.NewSpectralGrid(200, 1, 2301)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	const wienNMK = 2.897771955e-3 * 1e9 // metre kelvin, in nanometre kelvin

	for _, temperature := range []float64{3000, 4500, 5500, 7000, 10000} {
		shape, err := skybrightness.BlackbodyShape(grid, temperature)
		if err != nil {
			t.Fatalf("%g K: %v", temperature, err)
		}

		peak, at := 0.0, 0.0

		for i := range shape {
			if shape[i] > peak {
				peak, at = shape[i], float64(grid.At(i))
			}
		}

		want := wienNMK / temperature

		// One nanometre of grid resolution, plus a little for the peak being
		// flat near its top.
		if math.Abs(at-want) > 2 {
			t.Errorf("%g K peaks at %g nm, want %g from Wien displacement",
				temperature, at, want)
		}
	}
}

// A hotter source is bluer, everywhere, which is the property the shape exists
// to carry.
func TestBlackbodyShapeGetsBluerWithTemperature(t *testing.T) {
	t.Parallel()

	grid, err := unit.NewSpectralGrid(400, 5, 81)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	ratio := func(temperature float64) float64 {
		shape, err := skybrightness.BlackbodyShape(grid, temperature)
		if err != nil {
			t.Fatalf("%g K: %v", temperature, err)
		}

		// Blue end over red end.
		return shape[0] / shape[len(shape)-1]
	}

	cool, warm, hot := ratio(3500), ratio(5500), ratio(9000)

	if !(cool < warm && warm < hot) {
		t.Errorf("blue-to-red ratios are %.4f, %.4f, %.4f at 3500, 5500 and 9000 K; "+
			"a hotter source must put relatively more of its light in the blue",
			cool, warm, hot)
	}
}

// A shape that cannot be built is refused rather than returned as zeros.
func TestBlackbodyShapeRejectsBadInput(t *testing.T) {
	t.Parallel()

	grid, err := unit.NewSpectralGrid(400, 1, 401)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	for _, temperature := range []float64{0, -100, math.NaN(), math.Inf(1)} {
		if _, err := skybrightness.BlackbodyShape(grid, temperature); err == nil {
			t.Errorf("temperature %v was accepted", temperature)
		}
	}

	if _, err := skybrightness.BlackbodyShape(unit.SpectralGrid{}, 5500); err == nil {
		t.Error("an invalid grid was accepted")
	}
}

// Nothing overflows at the blue end of a cool source.
//
// The Planck exponent hc/(lambda k T) reaches 700 for a 300 K body around
// 68 nm and past 709 the exponential is an infinity. Grids here do not go that
// blue, but a caller's might, and an infinity in a spectral shape propagates
// into every band that touches it.
func TestBlackbodyShapeStaysFiniteAtTheBlueEnd(t *testing.T) {
	t.Parallel()

	grid, err := unit.NewSpectralGrid(10, 10, 200)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	shape, err := skybrightness.BlackbodyShape(grid, 300)
	if err != nil {
		t.Fatalf("BlackbodyShape: %v", err)
	}

	for i := range shape {
		if math.IsNaN(shape[i]) || math.IsInf(shape[i], 0) || shape[i] < 0 {
			t.Fatalf("slot %d at %v nm is %v", i, grid.At(i), shape[i])
		}
	}
}
