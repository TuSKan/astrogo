package magnitude_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/unit"
)

// raisedCosine builds a smooth band that reaches exactly zero at both ends
// with zero slope, so a quadrature over it converges cleanly.
//
// A top hat will not serve here. Its edges are discontinuous, so the
// trapezoid rule over-counts the first and last panel by half a step's worth
// of the integrand; that error mostly cancels between the numerator and the
// denominator but leaves a residue of a couple of tenths of a per cent that
// shrinks with the step. That residue is quadrature convergence on an
// artificial discontinuity, not a property of the projection, and no real
// response curve has it - Johnson V and Gaia G both taper.
func raisedCosine(name string, lo, hi float64, det magnitude.Detector) magnitude.Passband {
	const step = 0.5

	var (
		lambdas  []unit.WavelengthNM
		response []float64
	)

	centre, half := (lo+hi)/2, (hi-lo)/2

	for nm := lo; nm <= hi+1e-9; nm += step {
		lambdas = append(lambdas, unit.WavelengthNM(nm))
		response = append(response, 0.5*(1+math.Cos(math.Pi*(nm-centre)/half)))
	}

	return magnitude.Passband{
		Name:         name,
		WavelengthNM: lambdas,
		Response:     response,
		Detector:     det,
		Reference:    "synthetic test fixture, not a published curve",
	}
}

// A band-averaged quantity must not depend on how finely the grid it happens
// to be computed on is sampled.
//
// This is the failure this package is built to avoid, and the one the
// repository's own guidance singles out: normalising a spectral shape by the
// sum of its samples rather than by its integral ties the answer to the step.
// The result stays positive and plausible and changes every time a caller
// picks a different grid, so nothing downstream can detect it. The step
// cancels between the numerator and the denominator only if both are
// integrals, which is what this asserts.
//
// The steps below span a factor of sixteen. A sum-normalised implementation
// would be wrong by that factor; the tolerance is five parts in a hundred
// thousand.
func TestMeanFluxDensityIsIndependentOfGridSpacing(t *testing.T) {
	t.Parallel()

	// Sloped and curved, so a dependence on the step would show up rather than
	// cancel by symmetry.
	spectrumAt := func(nm float64) float64 {
		return 1e-8 * (1 + 0.5*math.Sin((nm-400)/60) + 0.002*(nm-500))
	}

	for _, det := range []struct {
		name string
		d    magnitude.Detector
	}{
		{"energy integrating", magnitude.EnergyIntegrating},
		{"photon counting", magnitude.PhotonCounting},
	} {
		band := raisedCosine("smooth", 450, 650, det.d)

		var reference float64

		for i, step := range []float64{4, 2, 1, 0.5, 0.25} {
			// 400 to 700 nm at every step, so the band is fully covered each
			// time and only the sampling changes.
			n := int(math.Round(300/step)) + 1

			grid, err := unit.NewSpectralGrid(400, unit.WavelengthNM(step), n)
			if err != nil {
				t.Fatalf("%s, step %g: NewSpectralGrid: %v", det.name, step, err)
			}

			spectrum := make([]float64, grid.Len())
			for j := range spectrum {
				spectrum[j] = spectrumAt(float64(grid.At(j)))
			}

			got, err := magnitude.MeanFluxDensity(spectrum, grid, band, 0.99)
			if err != nil {
				t.Fatalf("%s, step %g: %v", det.name, step, err)
			}

			if i == 0 {
				reference = got

				continue
			}

			if rel := math.Abs(got-reference) / reference; rel > 5e-5 {
				t.Errorf("%s: at a %g nm step the band mean is %.9g, against %.9g at 4 nm — "+
					"a relative change of %.3g, so the answer depends on the grid",
					det.name, step, got, reference, rel)
			}
		}
	}
}

// AB and ST are defined so their zero points coincide at 547.6 nm, so a band
// with its pivot there must give the same magnitude in both.
//
// The existing tests check each system against its own zero point, which
// cannot catch an error shared between them or one in the conversion that
// joins them. This checks the two against each other at a wavelength neither
// is derived from here: it exercises both zero points and the f_lambda to
// f_nu conversion at once, against a published number.
func TestABAndSTAgreeAtTheirDefinedCrossover(t *testing.T) {
	t.Parallel()

	// Narrow, so the pivot sits essentially at the centre of the band.
	band := topHat("crossover", 547.0, 548.2, magnitude.EnergyIntegrating)

	pivot, err := band.PivotWavelength()
	if err != nil {
		t.Fatalf("PivotWavelength: %v", err)
	}

	if p := float64(pivot); math.Abs(p-547.6) > 0.2 {
		t.Fatalf("the band's pivot is %.4f nm, so it is not the crossover band this test needs", p)
	}

	grid := mustGrid(t, 500, 101) // 500..600 nm at 1 nm

	spectrum := make([]float64, grid.Len())
	for i := range spectrum {
		spectrum[i] = 2e-7
	}

	ab, err := magnitude.SurfaceBrightness(spectrum, grid, band, magnitude.AB, 0.9)
	if err != nil {
		t.Fatalf("AB: %v", err)
	}

	st, err := magnitude.SurfaceBrightness(spectrum, grid, band, magnitude.ST, 0.9)
	if err != nil {
		t.Fatalf("ST: %v", err)
	}

	if math.Abs(ab-st) > 0.01 {
		t.Errorf("at a pivot of %.4f nm, AB = %.5f and ST = %.5f. The two systems are defined "+
			"to agree at 547.6 nm, so a gap of %.5f mag means a zero point or the "+
			"f_lambda to f_nu conversion between them is wrong", float64(pivot), ab, st, ab-st)
	}

	// Away from the crossover they must diverge in the direction the
	// definition requires: f_nu = f_lambda * lambda^2 / c, so redward of
	// 547.6 nm a fixed f_lambda is a larger f_nu, which is a brighter — that
	// is, smaller — AB magnitude than ST.
	red := topHat("red", 890, 910, magnitude.EnergyIntegrating)
	redGrid := mustGrid(t, 850, 101) // 850..950 nm

	redSpectrum := make([]float64, redGrid.Len())
	for i := range redSpectrum {
		redSpectrum[i] = 2e-7
	}

	abRed, err := magnitude.SurfaceBrightness(redSpectrum, redGrid, red, magnitude.AB, 0.9)
	if err != nil {
		t.Fatalf("AB in the red: %v", err)
	}

	stRed, err := magnitude.SurfaceBrightness(redSpectrum, redGrid, red, magnitude.ST, 0.9)
	if err != nil {
		t.Fatalf("ST in the red: %v", err)
	}

	if abRed >= stRed {
		t.Errorf("at 900 nm, AB = %.4f and ST = %.4f; redward of the crossover the same "+
			"f_lambda is a larger f_nu, so AB must be the brighter of the two", abRed, stRed)
	}

	// And the size of the gap is fixed by the ratio of the two pivots, since
	// both systems reduce to the same constant there.
	redPivot, err := red.PivotWavelength()
	if err != nil {
		t.Fatalf("PivotWavelength: %v", err)
	}

	want := -5 * math.Log10(float64(redPivot)/547.6)
	if got := abRed - stRed; math.Abs(got-want) > 0.02 {
		t.Errorf("AB - ST at a pivot of %.3f nm is %.5f, want %.5f from the crossover at 547.6 nm",
			float64(redPivot), got, want)
	}
}
