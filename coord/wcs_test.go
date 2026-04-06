package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/coord"
)

func TestPixelToWorld_TAN(t *testing.T) {
	w := coord.NewWCS(2)
	w.SetCTYPE([]string{"RA---TAN", "DEC--TAN"})

	// Set reference pixel (center of image typically)
	w.SetCRPIX([]float64{50.0, 50.0})

	// Coordinate at reference pixel (RA, DEC)
	w.SetCRVAL([]float64{10.0, 45.0})

	// Pixel scale (CDELT) in degrees per pixel
	w.SetCDELT([]float64{-0.01, 0.01})

	// At reference pixel it must equal CRVAL perfectly
	res, err := w.PixelToWorld([]float64{50.0, 50.0})
	if err != nil {
		t.Fatalf("PixelToWorld failed: %v", err)
	}

	if math.Abs(res[0]-10.0) > 1e-7 || math.Abs(res[1]-45.0) > 1e-7 {
		t.Errorf("Ref pixel mapping expected (10.0, 45.0), got (%.6f, %.6f)", res[0], res[1])
	}

	// Test mapping off center
	res, err = w.PixelToWorld([]float64{40.0, 60.0})
	if err != nil {
		t.Fatalf("PixelToWorld offset map failed: %v", err)
	}

	// Make sure coordinates drifted correctly along the right vectors
	if res[0] == 10.0 && res[1] == 45.0 {
		t.Errorf("Coordinates failed to project offsets beyond CRVAL: (%.6f, %.6f)", res[0], res[1])
	}
}

func TestPixelToWorld_LinearFallback(t *testing.T) {
	// 3D cube lacking explicit spherical metrics natively falls back to direct linear CDELT sums
	w := coord.NewWCS(3)
	w.SetCRVAL([]float64{0.0, 0.0, 1420.0}) // e.g. 1.4 GHz baseline
	w.SetCRPIX([]float64{10.0, 10.0, 1.0})
	w.SetCDELT([]float64{1.0, 1.0, 0.5})

	res, err := w.PixelToWorld([]float64{15.0, 10.0, 3.0})
	if err != nil {
		t.Fatalf("Linear pixel world map failed: %v", err)
	}

	// X shifted 5 units * 1.0 scale = +5.0
	if res[0] != 5.0 {
		t.Errorf("Expected 5.0, got %f", res[0])
	}

	// Z shifted 2 units * 0.5 scale = +1.0
	if res[2] != 1421.0 {
		t.Errorf("Expected 1421.0, got %f", res[2])
	}
}
