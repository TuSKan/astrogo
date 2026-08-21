package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
)

// A direction that is not on the sphere must not resolve to a pixel that is.
//
// The index comes from sin(latitude), and sine folds: sin(120 deg) equals
// sin(60 deg). So a latitude past the pole used to return the pixel at 60
// degrees on the *same* longitude, while "120 degrees of latitude" actually
// means 60 degrees at the longitude 180 away. The returned index was a real
// direction, passed every bounds check downstream, and was not the direction
// the caller named.
//
// Callers already test the index against the map length, so -1 fails that test
// and the mistake surfaces where it happened instead of becoming a radiance
// from the wrong part of the sky.
func TestPixelOfRejectsDirectionsOffTheSphere(t *testing.T) {
	t.Parallel()

	grid, err := coord.NewHEALPix(16)
	if err != nil {
		t.Fatalf("NewHEALPix: %v", err)
	}

	for _, lat := range []float64{90.0001, 100, 120, 180, 270, -90.0001, -120, -180} {
		if got := grid.PixelOf(angle.Deg(10), angle.Deg(lat)); got >= 0 {
			t.Errorf("latitude %+.4f returned pixel %d; it is not on the sphere", lat, got)
		}
	}

	for _, bad := range []angle.Angle{angle.Rad(math.NaN())} {
		if got := grid.PixelOf(angle.Deg(10), bad); got >= 0 {
			t.Errorf("a NaN latitude returned pixel %d", got)
		}

		if got := grid.PixelOf(bad, angle.Deg(10)); got >= 0 {
			t.Errorf("a NaN longitude returned pixel %d", got)
		}
	}
}

// The poles themselves are on the sphere and must still resolve.
//
// The guard is an inequality, so the boundary is where it would be got wrong.
func TestPixelOfAcceptsThePolesAndTheEquator(t *testing.T) {
	t.Parallel()

	grid, err := coord.NewHEALPix(16)
	if err != nil {
		t.Fatalf("NewHEALPix: %v", err)
	}

	for _, lat := range []float64{90, -90, 0, 89.9999, -89.9999} {
		for _, lon := range []float64{0, 90, 180, 270, 359.999, -45, 720} {
			got := grid.PixelOf(angle.Deg(lon), angle.Deg(lat))
			if got < 0 || got >= grid.NumPixels() {
				t.Errorf("lon %.3f lat %+.4f gave pixel %d, outside [0, %d)",
					lon, lat, got, grid.NumPixels())
			}
		}
	}
}

// Longitude wraps and latitude does not, which is the asymmetry the guard
// encodes. Adding a full turn of longitude must land on the same pixel.
func TestPixelOfWrapsLongitudeOnly(t *testing.T) {
	t.Parallel()

	grid, err := coord.NewHEALPix(16)
	if err != nil {
		t.Fatalf("NewHEALPix: %v", err)
	}

	for _, lat := range []float64{-60, -20, 0, 20, 60} {
		for _, lon := range []float64{0, 37, 180, 300} {
			base := grid.PixelOf(angle.Deg(lon), angle.Deg(lat))

			for _, turns := range []float64{-720, -360, 360, 720} {
				if got := grid.PixelOf(angle.Deg(lon+turns), angle.Deg(lat)); got != base {
					t.Errorf("lon %.0f%+.0f lat %+.0f gave pixel %d, want %d",
						lon, turns, lat, got, base)
				}
			}
		}
	}
}
