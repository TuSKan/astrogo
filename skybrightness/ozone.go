package skybrightness

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/unit"
)

// ozoneTransmission returns exp(-tau_O3 * xO3): the share of light at lambda
// that air's ozone column lets through along a path of xO3 ozone airmasses,
// from [atmosphere.OzoneAirmass].
//
// Ozone only absorbs, and it lies some 20 km up, above nearly all the air
// that scatters. Light from beyond the atmosphere crosses it once along the
// line of sight: starlight, the zodiacal and diffuse galactic light, the
// extragalactic background, and airglow, emitted near 90 km. The Moon's beam
// crosses it once on the way down, before it scatters below the layer.
// Artificial skyglow is made and scattered below it and never meets it.
// Until #632 no component applied it, and the scene's ozone column was read
// by nothing.
func ozoneTransmission(air *atmosphere.Atmosphere, lambda unit.WavelengthNM, xO3 float64) (float64, error) {
	tau, err := air.OzoneOpticalDepth(lambda)
	if err != nil {
		return 0, fmt.Errorf("ozone: %w", err)
	}

	return math.Exp(-float64(tau) * xO3), nil
}
