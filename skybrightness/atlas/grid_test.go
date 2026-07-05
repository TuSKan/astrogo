package atlas

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/skybrightness"
)

// northUpGrid builds a small north-up Grid covering a known extent, with 1°
// square pixels and the top-left corner of pixel (0,0) at (originLon,originLat).
func northUpGrid(width, height int, data []float64, originLon, originLat float64) *Grid {
	return &Grid{
		Width: width, Height: height, Data: data,
		GT: GeoTransform{A: originLon, B: 1, C: 0, D: originLat, E: 0, F: -1},
	}
}

// TestNewGridProviderInvalid verifies malformed grids are rejected.
func TestNewGridProviderInvalid(t *testing.T) {
	t.Parallel()

	cases := []*Grid{
		nil,
		{Width: 0, Height: 2, Data: []float64{1, 2}},
		{Width: 2, Height: 2, Data: []float64{1, 2}}, // wrong data length
	}

	for i, g := range cases {
		if _, err := NewGridProvider(g); !errors.Is(err, ErrInvalidGrid) {
			t.Errorf("case %d: expected ErrInvalidGrid, got %v", i, err)
		}
	}
}

// TestGridProviderConversion verifies the artificial mcd/m² → SQM conversion
// through the provider: a pixel at the natural-background luminance reads ~22.0.
func TestGridProviderConversion(t *testing.T) {
	t.Parallel()

	g := northUpGrid(1, 1, []float64{0.171168465}, -10, 40)

	p, err := NewGridProvider(g)
	if err != nil {
		t.Fatalf("NewGridProvider: %v", err)
	}

	// Centre of the single pixel.
	got, err := p.ZenithBrightness(40-0.5, -10+0.5)
	if err != nil {
		t.Fatalf("ZenithBrightness: %v", err)
	}

	testutil.AssertNear(t, "SQM", float64(got), 22.0, 1e-5)
}

// TestGridOutOfCoverage verifies a location outside the extent errors.
func TestGridOutOfCoverage(t *testing.T) {
	t.Parallel()

	g := northUpGrid(2, 2, []float64{1, 2, 3, 4}, 0, 0)

	p, _ := NewGridProvider(g)

	if _, err := p.ZenithBrightness(45, 45); !errors.Is(err, ErrOutOfCoverage) {
		t.Errorf("expected ErrOutOfCoverage, got %v", err)
	}
}

// TestGridNoData verifies that a fully no-data neighbourhood yields ErrNoData
// and that a partial no-data neighbourhood drops the missing corner.
func TestGridNoData(t *testing.T) {
	t.Parallel()

	nan := math.NaN()
	g := northUpGrid(2, 2, []float64{nan, nan, nan, nan}, 0, 0)

	p, _ := NewGridProvider(g)

	if _, err := p.ZenithBrightness(-0.5, 0.5); !errors.Is(err, ErrNoData) {
		t.Errorf("expected ErrNoData for all-NaN grid, got %v", err)
	}

	// Partial: one valid corner. Bilinear at the shared corner should equal it.
	g2 := northUpGrid(2, 2, []float64{10, nan, nan, nan}, 0, 0)
	p2, _ := NewGridProvider(g2)

	// Centre of pixel (0,0) is the only valid sample; query there.
	got, err := p2.ZenithBrightness(0-0.5, 0+0.5)
	if err != nil {
		t.Fatalf("partial no-data: %v", err)
	}

	testutil.AssertNear(t, "valid corner", float64(got), float64(skybrightness.SurfaceBrightnessFromMcdM2(10)), 1e-9)
}
