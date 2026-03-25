package render

import (
	"os"
	"testing"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/optics"
)

func TestDrawFOV(t *testing.T) {
	// Simulate query to ensure catalog works seamlessly natively
	target, err := catalog.Lookup("M42")
	if err != nil {
		t.Fatalf("unexpected catalog failure isolating M42 mathematically: %v", err)
	}
	
	// Create theoretical optics struct calculating the field arrays natively
	sensor := optics.Sensor{WidthMM: 35.8, HeightMM: 23.9, PixelPitch: 6.54}
	rig := optics.Optics{FocalLengthMM: 200.0, Extender: 2.0} // 400mm total explicit focal
	
	fovX, fovY := optics.CalculateFOV(sensor, rig)
	
	outPath := "test_fov.png"
	renderer := NewRenderer(DefaultConfig())
	err = renderer.DrawFOV(*target, fovX, fovY, outPath)
	if err != nil {
		t.Fatalf("DrawFOV strictly rejected rendering bounds natively: %v", err)
	}
	
	if _, err := os.Stat(outPath); os.IsNotExist(err) {
		t.Errorf("expected DrawFOV to flawlessly map the PNG disk structures natively")
	} else {
		os.Remove(outPath) // Cleanup dynamically generating artifacts cleanly
	}
}
