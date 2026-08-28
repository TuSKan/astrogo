package magnitude

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/unit"
)

// Luminance projects a spectral radiance through a luminous efficiency
// function, in candela per square metre.
//
//	L_v = K * INT L_e(lambda) * V(lambda) dlambda
//
// with L_e the spectral radiance in W m^-2 sr^-1 nm^-1, V the dimensionless
// efficiency function, and K its luminous efficacy in lumens per watt — 683
// for photopic vision, about 1700 for scotopic. The result is lm m^-2 sr^-1,
// which is a candela per square metre.
//
// # Why this is an integral and not a mean
//
// Because a luminance is a total, where a flux density is an average. Every
// other projection in this package divides by the band's own normalisation to
// answer "how bright per unit wavelength"; this one must not, because the eye
// sums what it receives across the whole band. Dividing here would produce a
// number that is smooth, positive, in plausible units and wrong by the width
// of whichever curve was passed in.
//
// # The efficacy is a parameter rather than a constant
//
// Because it belongs to the curve, and this package does not know which curve
// it was handed.
// [github.com/TuSKan/astrogo/skybrightness/dataset/luminosity.Vision.Efficacy]
// supplies the matching value; passing the photopic 683 with the scotopic
// curve is the one way to hold this wrong, and it is wrong by a factor of
// two and a half.
//
// # What it does not do
//
// Adapt. A luminance is the physical stimulus, not the sensation: whether an
// observer can see anything at it depends on their adaptation state, and the
// scotopic curve is only the right one once they are dark-adapted. This
// returns the stimulus.
func Luminance(
	spectrum []float64,
	grid unit.SpectralGrid,
	v Passband,
	efficacy, minCoverage float64,
) (unit.LuminanceCdM2, error) {
	if !(efficacy > 0) || math.IsInf(efficacy, 0) {
		return 0, fmt.Errorf("%w: luminous efficacy %g lm/W", ErrLuminousEfficacy, efficacy)
	}

	weights, coverage, err := v.Weights(grid)
	if err != nil {
		return 0, err
	}

	if coverage < minCoverage {
		return 0, fmt.Errorf("%w: %q covered %.4f of the band, need %.4f",
			ErrPassbandCoverage, v.Name, coverage, minCoverage)
	}

	if len(spectrum) != grid.Len() {
		return 0, fmt.Errorf("%w: %d spectrum samples, grid has %d",
			unit.ErrGridMismatch, len(spectrum), grid.Len())
	}

	weighted := make([]float64, grid.Len())
	for i := range weighted {
		weighted[i] = spectrum[i] * weights[i]
	}

	integral, err := grid.Integrate(weighted)
	if err != nil {
		return 0, fmt.Errorf("magnitude: luminance %q: %w", v.Name, err)
	}

	return unit.LuminanceCdM2(efficacy * integral), nil
}
