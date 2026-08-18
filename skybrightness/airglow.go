package skybrightness

import (
	"errors"
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/unit"
)

// ErrAirglowSpectrum is returned when the zenith spectrum does not match the
// destination grid.
var ErrAirglowSpectrum = errors.New("skybrightness: airglow zenith spectrum must be on the destination grid")

// Airglow accumulates the chemiluminescent emission of the upper atmosphere
// into dst, from a zenith spectrum and the viewing zenith angle.
//
//   - Model: Leinert et al. (1998) Eq. 13, the van Rhijn function, applied to
//     a caller-supplied zenith spectrum exactly as Masana et al. (2021)
//     Eq. 19-20 does.
//   - Emitting layer: [atmosphere.AirglowLayerHeightM] by default.
//
// # The spectrum is an input, not a prediction
//
// Airglow is the most variable term in a dark sky and the least predictable.
// Its emissions vary by up to 100 per cent night to night, with season, with
// the solar cycle, and with geographic and geomagnetic latitude — the OI
// lines at 630 and 636 nm are ionospheric and behave differently again from
// the mesospheric OH and Na D. Leinert et al. and Masana et al. both treat
// the zenith spectrum as a free parameter to be measured or supplied, and so
// does this function. A model that predicted it from nothing would be
// asserting far more than the literature supports.
//
// A reference spectrum can be computed from ESO's SkyCalc, which is what
// GAMBONS uses: Cerro Paranal at 2640 m, 350 to 1050 nm, with the monthly
// averaged solar radio flux set to 100 sfu for an average of one solar cycle.
// Scaling that spectrum is how a caller expresses a brighter or quieter night.
//
// The result carries [ClimatologicalAirglow] unless the caller's own flags say
// otherwise, because a supplied reference spectrum is climatology until it is
// tied to a measurement of the night in question.
func Airglow(
	dst SpectralRadiance,
	grid unit.SpectralGrid,
	zenith SpectralRadiance,
	zenithAngle angle.Angle,
	layerHeightM float64,
) (Flag, error) {
	if len(dst) != grid.Len() {
		return 0, fmt.Errorf("%w: %d destination slots, grid has %d",
			unit.ErrGridMismatch, len(dst), grid.Len())
	}

	if len(zenith) != grid.Len() {
		return 0, fmt.Errorf("%w: %d values, grid has %d", ErrAirglowSpectrum, len(zenith), grid.Len())
	}

	if layerHeightM <= 0 {
		layerHeightM = atmosphere.AirglowLayerHeightM
	}

	// Below the horizon there is no layer in the line of sight.
	if zenithAngle.Degrees() >= 90 {
		return ClimatologicalAirglow, nil
	}

	enhancement, err := atmosphere.VanRhijn(zenithAngle, layerHeightM)
	if err != nil {
		return 0, fmt.Errorf("skybrightness: airglow: %w", err)
	}

	flags := ClimatologicalAirglow

	// Leinert et al. note that extinction and scattering along the longer
	// path change the behaviour materially beyond about 40 degrees from the
	// zenith, and this applies the geometry alone.
	if zenithAngle.Degrees() > 40 {
		flags |= ExtrapolatedModel
	}

	for i := range dst {
		if zenith[i] < 0 {
			return 0, fmt.Errorf("%w: band %d is negative", ErrAirglowSpectrum, i)
		}

		dst[i] += zenith[i] * enhancement
	}

	return flags, nil
}
