package optics

import "math"

type Sensor struct {
	WidthMM    float64
	HeightMM   float64
	PixelPitch float64 // In micrometers (µm)
}

type Optics struct {
	FocalLengthMM float64
	Extender      float64 // Multiplier (e.g., 1.0 for none, 2.0 for a 2x teleconverter)
}

// CalculateFOV uses structural geometry tracking sensor limits to evaluate field of view structurally inherently.
// Returns observable degrees mapped directly natively.
func CalculateFOV(sensor Sensor, optics Optics) (fovXDegrees, fovYDegrees float64) {
	f := optics.FocalLengthMM * optics.Extender
	if f == 0 {
		return 0, 0
	}

	fovXDegrees = (180.0 / math.Pi) * 2.0 * math.Atan(sensor.WidthMM/(2*f))
	fovYDegrees = (180.0 / math.Pi) * 2.0 * math.Atan(sensor.HeightMM/(2*f))

	return fovXDegrees, fovYDegrees
}

// CalculateResolution performs strict topological array binding converting micrometers into arcsecs/px tracking.
func CalculateResolution(sensor Sensor, optics Optics) float64 {
	f := optics.FocalLengthMM * optics.Extender
	if f == 0 {
		return 0
	}

	// Uses the standard optical structural ratio translating to arcseconds smoothly
	return 206.265 * (sensor.PixelPitch / f)
}
