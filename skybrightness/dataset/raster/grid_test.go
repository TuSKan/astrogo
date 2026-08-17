package raster

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// northUpGrid builds a small north-up Grid with 1-degree square pixels and the
// top-left corner of pixel (0,0) at (originLon, originLat).
func northUpGrid(width, height int, data []float64, originLon, originLat float64) *Grid {
	return &Grid{
		Width: width, Height: height, Data: data,
		GT: GeoTransform{A: originLon, B: 1, C: 0, D: originLat, E: 0, F: -1},
	}
}

func TestGridValid(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		grid *Grid
		want bool
	}{
		{"nil", nil, false},
		{"good", northUpGrid(2, 2, []float64{1, 2, 3, 4}, 0, 0), true},
		{"short data", northUpGrid(2, 2, []float64{1, 2, 3}, 0, 0), false},
		{"zero width", northUpGrid(0, 2, nil, 0, 0), false},
		{"single row", northUpGrid(3, 1, []float64{1, 2, 3}, 0, 0), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := tc.grid.Valid(); got != tc.want {
				t.Errorf("Valid() = %v, want %v", got, tc.want)
			}
		})
	}

	// A malformed grid must say so on sampling rather than panic.
	bad := northUpGrid(2, 2, []float64{1}, 0, 0)
	if _, err := bad.SampleBilinear(0.5, -0.5); !errors.Is(err, ErrInvalidGrid) {
		t.Errorf("SampleBilinear on a malformed grid = %v, want ErrInvalidGrid", err)
	}
}

func TestGridOutOfCoverage(t *testing.T) {
	t.Parallel()

	g := northUpGrid(2, 2, []float64{1, 2, 3, 4}, 0, 0)

	if _, err := g.SampleBilinear(45, 45); !errors.Is(err, ErrOutOfCoverage) {
		t.Errorf("far outside the extent = %v, want ErrOutOfCoverage", err)
	}
}

// No-data is not zero. A raster of missing pixels must report that it has
// nothing rather than hand back a plausible-looking zero, which downstream
// would read as "measured darkness".
func TestGridNoData(t *testing.T) {
	t.Parallel()

	nan := math.NaN()

	all := northUpGrid(2, 2, []float64{nan, nan, nan, nan}, 0, 0)
	if _, err := all.SampleBilinear(0.5, -0.5); !errors.Is(err, ErrNoData) {
		t.Errorf("all-NaN grid = %v, want ErrNoData", err)
	}

	// A declared sentinel counts too, not just NaN.
	sentinel := northUpGrid(2, 2, []float64{-9999, -9999, -9999, -9999}, 0, 0)
	sentinel.NoData, sentinel.HasNoData = -9999, true

	if _, err := sentinel.SampleBilinear(0.5, -0.5); !errors.Is(err, ErrNoData) {
		t.Errorf("all-sentinel grid = %v, want ErrNoData", err)
	}

	// One valid corner: sampling at its centre returns it exactly, with the
	// missing neighbours dropped rather than treated as zero.
	partial := northUpGrid(2, 2, []float64{10, nan, nan, nan}, 0, 0)

	got, err := partial.SampleBilinear(0.5, -0.5)
	if err != nil {
		t.Fatalf("partial no-data: %v", err)
	}

	testutil.AssertNear(t, "sole valid corner", got, 10, 1e-9)
}

func TestGridAt(t *testing.T) {
	t.Parallel()

	g := northUpGrid(2, 2, []float64{1, 2, 3, 4}, 0, 0)

	if v, ok := g.At(1, 1); !ok || v != 4 {
		t.Errorf("At(1,1) = (%v, %v), want (4, true)", v, ok)
	}

	for _, p := range [][2]int{{-1, 0}, {0, -1}, {2, 0}, {0, 2}} {
		if _, ok := g.At(p[0], p[1]); ok {
			t.Errorf("At%v reported an in-bounds sample", p)
		}
	}
}

// Pixel centres sit half a pixel in from the corner the transform names.
func TestGridLonLat(t *testing.T) {
	t.Parallel()

	g := northUpGrid(2, 2, []float64{1, 2, 3, 4}, 10, 50)

	lon, lat := g.LonLat(0, 0)
	testutil.AssertNear(t, "centre lon", lon, 10.5, 1e-12)
	testutil.AssertNear(t, "centre lat", lat, 49.5, 1e-12)

	lon, lat = g.LonLat(1, 1)
	testutil.AssertNear(t, "next centre lon", lon, 11.5, 1e-12)
	testutil.AssertNear(t, "next centre lat", lat, 48.5, 1e-12)
}

// Bilinear interpolation must reproduce the corner values exactly and give the
// mean at the midpoint between them.
func TestGridBilinear(t *testing.T) {
	t.Parallel()

	g := northUpGrid(2, 2, []float64{0, 10, 20, 30}, 0, 0)

	// Pixel centres.
	for _, tc := range []struct {
		lon, lat, want float64
	}{
		{0.5, -0.5, 0},
		{1.5, -0.5, 10},
		{0.5, -1.5, 20},
		{1.5, -1.5, 30},
	} {
		got, err := g.SampleBilinear(tc.lon, tc.lat)
		if err != nil {
			t.Fatalf("SampleBilinear(%v, %v): %v", tc.lon, tc.lat, err)
		}

		testutil.AssertNear(t, "pixel centre", got, tc.want, 1e-9)
	}

	// Halfway between the two top pixels.
	got, err := g.SampleBilinear(1.0, -0.5)
	if err != nil {
		t.Fatalf("SampleBilinear: %v", err)
	}

	testutil.AssertNear(t, "midpoint", got, 5, 1e-9)
}
