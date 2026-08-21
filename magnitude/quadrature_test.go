package magnitude_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/unit"
)

// auditSpectrum is the sloped, curved spectrum the grid-independence test uses,
// shared so both tests are asking about the same integrand.
func auditSpectrum(nm float64) float64 {
	return 1e-8 * (1 + 0.5*math.Sin((nm-400)/60) + 0.002*(nm-500))
}

// A band mean over a top hat converges to the true value at first order in the
// step, which is what distinguishes quadrature error from a wrong answer.
//
// This exists because the distinction is easy to get wrong, and was. A
// grid-independence check over a rectangular band shows the answer moving by a
// quarter of a per cent between a 4 nm step and a 0.25 nm one, which looks
// exactly like the failure that check is for — a mean normalised by the sample
// count rather than by the integral, tied to the grid. It is not. The give-away
// is the direction of travel: a sum-normalised mean would scale with the step,
// by the full factor of sixteen across that range, while this converges, and
// converges on the analytic value.
//
// It converges at first order rather than second because a top hat's edges are
// discontinuous. The trapezoid rule is second order on a smooth integrand and
// drops to first at a jump, since the panel straddling the edge is wrong by an
// amount proportional to the step no matter how fine the grid. Halving the step
// halves the error, exactly, which is the signature asserted below.
//
// TestMeanFluxDensityIsIndependentOfGridSpacing uses a smooth band for its own
// tolerance precisely so that it is measuring the projection rather than this.
func TestBandMeanConvergesOnTheAnalyticValue(t *testing.T) {
	t.Parallel()

	const (
		lo = 480.0
		hi = 620.0
	)

	band := topHat("tophat", lo, hi, magnitude.EnergyIntegrating)

	// The true band mean, by Simpson's rule at a resolution far finer than any
	// grid below, computed here rather than taken from the code under test.
	want := simpsonMean(lo, hi, 2_000_000)

	var previous float64

	for i, step := range []float64{4, 2, 1, 0.5, 0.25} {
		grid, err := unit.NewSpectralGrid(400, unit.WavelengthNM(step), int(math.Round(300/step))+1)
		if err != nil {
			t.Fatalf("step %g: %v", step, err)
		}

		spectrum := make([]float64, grid.Len())
		for j := range spectrum {
			spectrum[j] = auditSpectrum(float64(grid.At(j)))
		}

		got, err := magnitude.MeanFluxDensity(spectrum, grid, band, 0.9)
		if err != nil {
			t.Fatalf("step %g: %v", step, err)
		}

		err2 := math.Abs(got-want) / want

		// Every step must be closer to the analytic value than the last, which
		// is what a sum-normalised mean would not do.
		if i > 0 {
			ratio := previous / err2

			if ratio < 1.8 || ratio > 2.2 {
				t.Errorf("halving the step to %g reduced the error by a factor of %.3f, want 2 — "+
					"first-order convergence is the signature of the trapezoid rule at the band's "+
					"discontinuous edges; a different order means something else is happening",
					step, ratio)
			}
		}

		previous = err2
	}

	// And the finest grid is genuinely close, so the sequence is converging on
	// the right number rather than merely converging.
	if previous > 2e-4 {
		t.Errorf("at the finest step the band mean is still %.3g away from the analytic value", previous)
	}
}

// simpsonMean is the exact band mean of auditSpectrum over a unit top hat,
// independent of anything in magnitude.
func simpsonMean(lo, hi float64, steps int) float64 {
	h := (hi - lo) / float64(steps)
	sum := auditSpectrum(lo) + auditSpectrum(hi)

	for i := 1; i < steps; i++ {
		w := 2.0
		if i%2 == 1 {
			w = 4.0
		}

		sum += w * auditSpectrum(lo+float64(i)*h)
	}

	return (sum * h / 3) / (hi - lo)
}
