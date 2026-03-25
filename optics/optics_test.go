package optics

import (
	"math"
	"testing"
)

// TestCalculateFOV enforces mathematically standard benchmarks ensuring native structural alignment maps explicitly continuously.
func TestCalculateFOV(t *testing.T) {
	// Native testing logic tracking Canon 6D boundaries mapping structured pixels explicitly.
	sensor := Sensor{
		WidthMM:    35.8,
		HeightMM:   23.9,
		PixelPitch: 6.54,
	}

	opticsStruct := Optics{
		FocalLengthMM: 200.0,
		Extender:      2.0,
	}

	fovX, fovY := CalculateFOV(sensor, opticsStruct)
	resolution := CalculateResolution(sensor, opticsStruct)

	// Validate algorithm structural outputs securely evaluating math boundaries
	effectiveF := 400.0
	expectedFovX := (180.0 / math.Pi) * 2.0 * math.Atan(sensor.WidthMM/(2*effectiveF))
	expectedFovY := (180.0 / math.Pi) * 2.0 * math.Atan(sensor.HeightMM/(2*effectiveF))
	expectedRes := 206.265 * (sensor.PixelPitch / effectiveF)

	if math.Abs(fovX-expectedFovX) > 1e-9 {
		t.Errorf("Expected FOV X internally %f degrees, got %f bounds explicitly", expectedFovX, fovX)
	}

	if math.Abs(fovY-expectedFovY) > 1e-9 {
		t.Errorf("Expected FOV Y internally %f degrees, got %f bounds explicitly natively", expectedFovY, fovY)
	}

	if math.Abs(resolution-expectedRes) > 1e-9 {
		t.Errorf("Expected pixel density extraction evaluating natively at %f arcsecs/px, mapping %f inherently", expectedRes, resolution)
	}
}
