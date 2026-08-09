package skybrightness

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/unit"
)

// ErrInvalidInstrument is returned by Instrument.Validate.
var ErrInvalidInstrument = errors.New("skybrightness: invalid Instrument")

// Sentinel components ErrInvalidInstrument wraps, so a caller can
// errors.Is against the specific violation.
var (
	errInstrumentAperture   = errors.New("Instrument: ApertureM must be > 0")
	errInstrumentThroughput = errors.New("Instrument: Throughput must be in (0,1]")
	errInstrumentQE         = errors.New("Instrument: QuantumEfficiency must be in (0,1]")
	errInstrumentPixelScale = errors.New("Instrument: PixelScaleArcsec must be > 0")
)

// Instrument describes the telescope+detector combination
// DeriveDetectorBackground uses to turn a passband-integrated sky
// background into a detector background rate. Deliberately NOT embedded
// in the atmospheric/emission engine — the mandate is explicit that
// telescope assumptions must not leak into the physics.
//
// The background-rate model here is a simplified, documented
// approximation (aperture area x throughput x QE x pixel solid angle),
// not a full instrument-simulator treatment (no central obstruction, no
// vignetting, no plate-scale distortion). Refining it is future work; it
// is not fabricated as more precise than it is.
type Instrument struct {
	ApertureM         float64 // effective clear aperture diameter, metres
	Throughput        float64 // optical throughput, [0,1]
	QuantumEfficiency float64 // detector QE, [0,1]
	PixelScaleArcsec  float64 // arcsec per pixel
}

// Validate checks that every field is physically plausible.
func (i Instrument) Validate() error {
	switch {
	case i.ApertureM <= 0:
		return fmt.Errorf("%w: %w", ErrInvalidInstrument, errInstrumentAperture)
	case i.Throughput <= 0 || i.Throughput > 1:
		return fmt.Errorf("%w: %w", ErrInvalidInstrument, errInstrumentThroughput)
	case i.QuantumEfficiency <= 0 || i.QuantumEfficiency > 1:
		return fmt.Errorf("%w: %w", ErrInvalidInstrument, errInstrumentQE)
	case i.PixelScaleArcsec <= 0:
		return fmt.Errorf("%w: %w", ErrInvalidInstrument, errInstrumentPixelScale)
	default:
		return nil
	}
}

// BackgroundRate converts a passband-integrated sky photon radiance into a
// per-pixel detector background rate: aperture collecting area x pixel
// solid angle x throughput x quantum efficiency.
func (i Instrument) BackgroundRate(sky unit.PhotonRadiance) unit.ElectronsPerPixelPerSecond {
	apertureArea := math.Pi * (i.ApertureM / 2) * (i.ApertureM / 2)                   // m^2
	pixelRad := i.PixelScaleArcsec * math.Sqrt(constants.ArcsecondSquaredToSteradian) // pixel scale, radians
	pixelSolidAngle := pixelRad * pixelRad                                            // steradians

	return unit.ElectronsPerPixelPerSecond(float64(sky) * apertureArea * pixelSolidAngle * i.Throughput * i.QuantumEfficiency)
}
