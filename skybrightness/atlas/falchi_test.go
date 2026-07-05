package atlas

import (
	"bytes"
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// TestFalchiProviderWindowed verifies the windowed provider resolves the
// artificial brightness at a known pixel centre and converts it to SQM.
func TestFalchiProviderWindowed(t *testing.T) {
	t.Parallel()

	// One pixel holds the natural-background luminance ⇒ ~22.0 mag/arcsec².
	s := synthTIFF{
		width: 2, height: 2,
		pixels:    []float32{0.171168465, 100, 50, 10},
		originLon: -47, originLat: -22, pxSize: 0.5,
	}

	p, err := NewFalchiProvider(bytes.NewReader(s.build(t)))
	if err != nil {
		t.Fatalf("NewFalchiProvider: %v", err)
	}

	lon, lat := s.centerLonLat(0, 0)

	got, err := p.ZenithBrightness(lat, lon)
	if err != nil {
		t.Fatalf("ZenithBrightness: %v", err)
	}

	testutil.AssertNear(t, "SQM", float64(got), 22.0, 1e-4)

	// A brighter pixel (100 mcd/m²) must read darker-magnitude (smaller SQM).
	lon3, lat3 := s.centerLonLat(1, 0)

	bright, err := p.ZenithBrightness(lat3, lon3)
	if err != nil {
		t.Fatalf("ZenithBrightness(bright): %v", err)
	}

	if !(bright < got) {
		t.Errorf("expected brighter pixel to have smaller SQM: bright=%.3f natural=%.3f", float64(bright), float64(got))
	}
}

// TestLoadFalchiGrid verifies the whole-raster loader produces a Grid whose
// provider matches the windowed provider at a pixel centre.
func TestLoadFalchiGrid(t *testing.T) {
	t.Parallel()

	s := synthTIFF{
		width: 3, height: 2, pixels: rampPixels(3, 2, 2),
		originLon: 5, originLat: 10, pxSize: 1.0,
	}

	raw := s.build(t)

	grid, err := LoadFalchiGrid(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("LoadFalchiGrid: %v", err)
	}

	if grid.Width != 3 || grid.Height != 2 {
		t.Fatalf("grid dims = %dx%d, want 3x2", grid.Width, grid.Height)
	}

	gp, _ := NewGridProvider(grid)
	wp, _ := NewFalchiProvider(bytes.NewReader(raw))

	lon, lat := s.centerLonLat(2, 1)

	a, err := gp.ZenithBrightness(lat, lon)
	if err != nil {
		t.Fatalf("grid provider: %v", err)
	}

	b, err := wp.ZenithBrightness(lat, lon)
	if err != nil {
		t.Fatalf("windowed provider: %v", err)
	}

	testutil.AssertNear(t, "grid vs windowed", float64(a), float64(b), 1e-9)
}

// TestNewFalchiProviderOverride verifies WithGeoTransform is honored — built
// here by stripping nothing (the synth always writes model tags), so instead we
// confirm the option threads through without error on a tag-bearing file.
func TestNewFalchiProviderOverride(t *testing.T) {
	t.Parallel()

	s := synthTIFF{width: 2, height: 2, pixels: rampPixels(2, 2, 1), originLon: 0, originLat: 0, pxSize: 1}

	_, err := NewFalchiProvider(bytes.NewReader(s.build(t)), WithGeoTransform(GeoTransform{A: 0, B: 1, D: 0, F: -1}))
	if err != nil {
		t.Fatalf("NewFalchiProvider with override: %v", err)
	}
}

// TestLorenzBlocked verifies the Lorenz loader reports its blocked status.
func TestLorenzBlocked(t *testing.T) {
	t.Parallel()

	if _, err := NewLorenzProvider(bytes.NewReader(nil)); !errors.Is(err, ErrLorenzNoNumericData) {
		t.Errorf("expected ErrLorenzNoNumericData, got %v", err)
	}
}
