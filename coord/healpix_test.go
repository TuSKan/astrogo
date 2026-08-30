package coord_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
)

func healpix(t *testing.T, nside int64) coord.HEALPix {
	t.Helper()

	h, err := coord.NewHEALPix(nside)
	if err != nil {
		t.Fatalf("NewHEALPix(%d): %v", nside, err)
	}

	return h
}

func TestHEALPixResolution(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct{ nside, npix int64 }{
		{1, 12}, {2, 48}, {4, 192}, {8, 768}, {16, 3072}, {256, 786432},
	} {
		if got := healpix(t, tc.nside).NumPixels(); got != tc.npix {
			t.Errorf("nside %d: NumPixels = %d, want %d", tc.nside, got, tc.npix)
		}
	}

	// GAMBONS publishes at HEALPix *order* 8, which is nside = 2^8 = 256:
	// 786432 pixels of 1.5979e-5 sr. Both figures are quoted independently
	// in Masana et al. (2021), and "resolution 8" meaning order rather than
	// nside is the trap — nside 8 would be a thousand times coarser.
	h := healpix(t, 256)
	if got := h.NumPixels(); got != 786432 {
		t.Errorf("nside 256: NumPixels = %d, want 786432", got)
	}

	if got := h.PixelArea(); math.Abs(got-1.5979e-5)/1.5979e-5 > 1e-4 {
		t.Errorf("nside 256: PixelArea = %.6g sr, want 1.5979e-5", got)
	}

	// The areas must sum to the whole sphere exactly.
	if total := h.PixelArea() * float64(h.NumPixels()); math.Abs(total-4*math.Pi) > 1e-12 {
		t.Errorf("pixel areas sum to %v, want 4*pi", total)
	}
}

func TestNewHEALPixRejectsBadNside(t *testing.T) {
	t.Parallel()

	for _, nside := range []int64{0, -1, 3, 5, 6, 100, 1 << 30} {
		if _, err := coord.NewHEALPix(nside); !errors.Is(err, coord.ErrHEALPixNside) {
			t.Errorf("nside %d: err = %v, want ErrHEALPixNside", nside, err)
		}
	}
}

// The strongest self-consistency check available: every pixel's centre must
// map back to that same pixel. It exercises the forward and inverse
// transforms, the face assignment, and the Morton interleaving against each
// other across all twelve faces and both polar caps.
func TestHEALPixRoundTrip(t *testing.T) {
	t.Parallel()

	for _, nside := range []int64{1, 2, 4, 8, 16, 32} {
		h := healpix(t, nside)

		for pixel := range h.NumPixels() {
			lon, lat, err := h.Center(pixel)
			if err != nil {
				t.Fatalf("nside %d: Center(%d): %v", nside, pixel, err)
			}

			if got := h.PixelOf(lon, lat); got != pixel {
				t.Fatalf("nside %d: pixel %d has centre (%.6f, %.6f) which maps to pixel %d",
					nside, pixel, lon.Degrees(), lat.Degrees(), got)
			}
		}
	}
}

// Equal area is HEALPix's defining property and the reason a radiance map
// can store one value per pixel with no weighting. Scattering points
// uniformly over the sphere must fill the pixels uniformly — a tessellation
// with unequal cells would show it immediately as a spread far beyond
// Poisson noise.
func TestHEALPixPixelsAreEqualArea(t *testing.T) {
	t.Parallel()

	h := healpix(t, 4)
	npix := h.NumPixels()

	counts := make([]int, npix)

	// A deterministic quasi-uniform sweep: uniform in longitude and in
	// sin(latitude), which is uniform on the sphere.
	const steps = 400

	for i := range steps {
		z := -1 + 2*(float64(i)+0.5)/steps
		lat := angle.Rad(math.Asin(z))

		for j := range steps {
			lon := angle.Rad(2 * math.Pi * (float64(j) + 0.5) / steps)
			counts[h.PixelOf(lon, lat)]++
		}
	}

	expect := float64(steps*steps) / float64(npix)

	for pixel, n := range counts {
		if rel := math.Abs(float64(n)-expect) / expect; rel > 0.05 {
			t.Errorf("pixel %d holds %d samples, want about %.0f (%.1f%% off)",
				pixel, n, expect, rel*100)
		}
	}
}

// Every direction on the sphere must land in a valid pixel — including the
// poles and the longitude wrap, which is where an index scheme built on
// floor() and bit masks is most likely to fall off its range.
func TestHEALPixCoversTheSphere(t *testing.T) {
	t.Parallel()

	h := healpix(t, 8)

	cases := []struct{ lon, lat float64 }{
		{0, 90}, {0, -90}, {0, 0}, {360, 0}, {-180, 0}, {180, 0},
		{0, 89.999999}, {0, -89.999999}, {359.999999, 41.81}, {0.000001, -41.81},
		{123.456, 66.6}, {270, -66.6},
	}

	for _, tc := range cases {
		pixel := h.PixelOf(angle.Deg(tc.lon), angle.Deg(tc.lat))
		if pixel < 0 || pixel >= h.NumPixels() {
			t.Errorf("(%v, %v) gave pixel %d, outside [0, %d)", tc.lon, tc.lat, pixel, h.NumPixels())
		}
	}
}

// A longitude offset by a full turn is the same direction and must give the
// same pixel; the wrap is otherwise an easy place to lose a face.
//
// The longitudes here deliberately avoid exact multiples of 45 degrees. Those
// lie on the base faces' own boundaries, where wrapping through 720 degrees
// and back leaves a one-ulp residue that legitimately falls on either side.
// Testing there would be asserting a tie-break no implementation guarantees,
// not asserting the wrap.
func TestHEALPixLongitudeWrap(t *testing.T) {
	t.Parallel()

	h := healpix(t, 16)

	for _, lat := range []float64{-80, -30, 0, 30, 80} {
		for _, lon := range []float64{7.5, 44.9, 123.4, 200, 358.7} {
			base := h.PixelOf(angle.Deg(lon), angle.Deg(lat))

			for _, turn := range []float64{-720, -360, 360, 720} {
				if got := h.PixelOf(angle.Deg(lon+turn), angle.Deg(lat)); got != base {
					t.Errorf("(%v+%v, %v) gave pixel %d, want %d", lon, turn, lat, got, base)
				}
			}
		}
	}
}

// The nested ordering is a quadtree: a pixel at nside N contains exactly the
// four pixels 4p..4p+3 at nside 2N. That hierarchy is what lets a map be
// degraded or refined by integer arithmetic alone, and it only holds if the
// Morton interleaving is right.
func TestHEALPixNestedHierarchy(t *testing.T) {
	t.Parallel()

	coarse := healpix(t, 4)
	fine := healpix(t, 8)

	for pixel := range fine.NumPixels() {
		lon, lat, err := fine.Center(pixel)
		if err != nil {
			t.Fatalf("Center(%d): %v", pixel, err)
		}

		if got, want := coarse.PixelOf(lon, lat), pixel/4; got != want {
			t.Fatalf("fine pixel %d sits in coarse pixel %d, want %d", pixel, got, want)
		}
	}
}

func TestHEALPixCenterRejectsBadPixel(t *testing.T) {
	t.Parallel()

	h := healpix(t, 4)

	for _, pixel := range []int64{-1, h.NumPixels(), h.NumPixels() + 100} {
		if _, _, err := h.Center(pixel); !errors.Is(err, coord.ErrHEALPixPixel) {
			t.Errorf("Center(%d): err = %v, want ErrHEALPixPixel", pixel, err)
		}
	}
}

func BenchmarkHEALPixPixelOf(b *testing.B) {
	h, err := coord.NewHEALPix(256)
	if err != nil {
		b.Fatal(err)
	}

	lon, lat := angle.Deg(123.4), angle.Deg(-42.1)

	b.ReportAllocs()

	for b.Loop() {
		_ = h.PixelOf(lon, lat)
	}
}
