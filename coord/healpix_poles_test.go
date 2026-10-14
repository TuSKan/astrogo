package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
)

// The polar caps at every resolution NewHEALPix accepts (#778).
//
// Near a pole sin(latitude) rounds to ±1. PixelOf took 1 - |sin(latitude)|,
// which cancels to nothing within milliarcseconds of the pole, and Center
// returned the arcsine of a z that had rounded to 1. Neither shows at the
// resolutions the other tests use: at nside 2^29 Center put the first rings
// on the pole itself, up to 4.8 pixel widths from where they are, and PixelOf
// gave the wrong pixel for 246 of 1,830 ring centers.
//
// The expected latitude is closed-form and independent of either function's
// arithmetic. A ring nr from the pole in a polar cap sits at
// 1 - cos(theta) = nr^2/(3 nside^2), so theta = 2 asin(nr/(sqrt(6) nside)),
// which keeps its precision where theta is small.

// nestedIndex interleaves x and y bit by bit, x in the even bits: the NESTED
// index of (x, y) within a base face.
func nestedIndex(x, y int64) int64 {
	var n int64

	for bit := range 30 {
		n |= (x >> bit & 1) << (2 * bit)
		n |= (y >> bit & 1) << (2*bit + 1)
	}

	return n
}

// polarRingPixel is the k-th pixel, k < nr, of ring nr counted from the north
// pole in base face 0, or from the south pole in base face 8, and that ring's
// latitude.
func polarRingPixel(nside, nr, k int64, north bool) (pixel int64, lat float64) {
	theta := 2 * math.Asin(float64(nr)/(math.Sqrt(6)*float64(nside)))

	if north {
		// Face 0 holds the north-cap ring nr at ix + iy = 2 nside - 1 - nr.
		return nestedIndex(nside-1-k, nside-nr+k), math.Pi/2 - theta
	}

	// Face 8 holds the south-cap ring nr at ix + iy = nr - 1.
	return 8*nside*nside + nestedIndex(k, nr-1-k), theta - math.Pi/2
}

func TestHEALPixKeepsThePolesAtEveryResolution(t *testing.T) {
	t.Parallel()

	for _, order := range []int{8, 16, 24, 26, 27, 28, 29} {
		nside := int64(1) << order
		h := healpix(t, nside)
		width := math.Sqrt(h.PixelArea())

		worst := 0.0

		for _, north := range []bool{true, false} {
			for nr := int64(1); nr <= 40; nr++ {
				for k := range nr {
					pixel, lat := polarRingPixel(nside, nr, k, north)

					lon, gotLat, err := h.Center(pixel)
					if err != nil {
						t.Fatalf("nside 2^%d: Center(%d): %v", order, pixel, err)
					}

					worst = max(worst, math.Abs(gotLat.Radians()-lat)/width)

					if got := h.PixelOf(lon, angle.Rad(lat)); got != pixel {
						t.Fatalf("nside 2^%d, ring %d from the %s pole: PixelOf(%.12f°, %.15f°) = %d, want %d",
							order, nr, poleName(north), lon.Degrees(), angle.Rad(lat).Degrees(), got, pixel)
					}

					if got := h.PixelOf(lon, gotLat); got != pixel {
						t.Fatalf("nside 2^%d: pixel %d's center (%.12f°, %.15f°) maps back to %d",
							order, pixel, lon.Degrees(), gotLat.Degrees(), got)
					}
				}
			}
		}

		t.Logf("nside 2^%d: worst center latitude error %.3g pixel widths", order, worst)

		if worst > 0.001 {
			t.Errorf("nside 2^%d: a ring center's latitude is %.3g pixel widths from its closed form", order, worst)
		}
	}
}

func poleName(north bool) string {
	if north {
		return "north"
	}

	return "south"
}
