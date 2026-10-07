//go:build network

package plan_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/skybrightness/dataset/passband"
	"github.com/TuSKan/astrogo/unit"
)

// VisibleTonight dims every V magnitude by one coefficient, its air's
// extinction at 547.8 nm, V's pivot wavelength. That stands in for the extinction averaged over V's
// passband, which is what a broadband magnitude actually loses, and this
// measures how well.
//
// The passband is SVO's Generic/Bessell.V, the profile the skybrightness
// tests establish as the Johnson-Cousins family catalogs use. Its pivot
// wavelength comes from magnitude.Passband.PivotWavelength, which honors the
// band's energy-counting detector. The band average is the transmission a
// flat-spectrum source sees through the passband, in VisibleTonight's default
// air at sea level, at Paranal's 2,640 m and at 4,000 m.
//
// The bound is 0.001 mag per airmass, a tenth of the 0.01 the model itself is
// validated to at Paranal. Measured: 0.0007 at sea level, less higher up.
func TestVExtinctionAtThePivotIsTheBandAverage(t *testing.T) {
	const bound = 0.001 // mag per airmass

	band, err := passband.Fetch(t.Context(), "Generic/Bessell.V")
	testutil.SkipOnUpstreamFailure(t, err)

	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	pivot, err := band.PivotWavelength()
	if err != nil {
		t.Fatalf("PivotWavelength: %v", err)
	}

	if math.Abs(float64(pivot)-547.8) > 0.05 {
		t.Errorf("Bessell V's pivot wavelength is %.2f nm; VisibleTonight evaluates V at 547.8", float64(pivot))
	}

	lo, hi := band.Span()
	start := math.Floor(float64(lo))

	grid, err := unit.NewSpectralGrid(unit.WavelengthNM(start), 1, int(math.Ceil(float64(hi))-start)+1)
	if err != nil {
		t.Fatalf("NewSpectralGrid: %v", err)
	}

	for _, heightM := range []float64{0, 2640, 4000} {
		air := defaultNight(t, unit.Meters(heightM))

		transmission := make([]float64, grid.Len())

		for i := range transmission {
			k, err := air.Extinction(grid.At(i))
			if err != nil {
				t.Fatalf("Extinction at %v nm: %v", grid.At(i), err)
			}

			transmission[i] = math.Pow(10, -0.4*k)
		}

		mean, err := magnitude.MeanFluxDensity(transmission, grid, band, 0.99)
		if err != nil {
			t.Fatalf("MeanFluxDensity: %v", err)
		}

		averaged := -2.5 * math.Log10(mean)

		atPivot, err := air.Extinction(547.8)
		if err != nil {
			t.Fatalf("Extinction at 547.8 nm: %v", err)
		}

		if diff := averaged - atPivot; math.Abs(diff) > bound {
			t.Errorf("at %g m: V's band-averaged extinction is %.4f, the value at the pivot %.4f, "+
				"%+.4f apart; the bound is %g", heightM, averaged, atPivot, diff, bound)
		}

		t.Logf("%5g m: band average %.4f, at the pivot %.4f (%+.4f); pivot %.2f nm",
			heightM, averaged, atPivot, averaged-atPivot, float64(pivot))
	}
}
