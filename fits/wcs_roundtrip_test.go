package fits

import (
	"math"
	"testing"
)

// tanWCS builds an ordinary gnomonic header: reference sky position, reference
// pixel, and a tenth of a degree per pixel.
func tanWCS(t *testing.T) *WCS {
	t.Helper()

	h := NewHeader()
	h.Append(Card{Keyword: "NAXIS", Value: "2"})
	h.Append(Card{Keyword: "CTYPE1", Value: "RA---TAN"})
	h.Append(Card{Keyword: "CTYPE2", Value: "DEC--TAN"})
	h.Append(Card{Keyword: "CRVAL1", Value: "10.0"})
	h.Append(Card{Keyword: "CRVAL2", Value: "20.0"})
	h.Append(Card{Keyword: "CRPIX1", Value: "100.0"})
	h.Append(Card{Keyword: "CRPIX2", Value: "200.0"})
	h.Append(Card{Keyword: "CDELT1", Value: "-0.1"})
	h.Append(Card{Keyword: "CDELT2", Value: "0.1"})

	wcs, err := ExtractWCS(h)
	if err != nil {
		t.Fatalf("ExtractWCS: %v", err)
	}

	return wcs
}

// A pixel carried to the sky and back must return to itself.
//
// PixelToWorld and WorldToPixel are inverses by contract: one calls deproject
// and the other project, through the same linear transform. Nothing tested
// that they agree — wcs_test.go covers header extraction only — and they do
// not. The two halves are written in opposite conventions, so a position east
// and north of the reference comes back west and south of it: a 180 degree
// rotation about the reference pixel, which is a finite, ordinary coordinate
// somewhere real.
//
// The reference pixel itself round-trips perfectly, which is why an
// eyeball check at the centre of an image would not show this.
func TestPixelWorldRoundTrip(t *testing.T) {
	t.Parallel()

	wcs := tanWCS(t)

	for _, dx := range []float64{-40, -10, -1, 0, 1, 10, 40} {
		for _, dy := range []float64{-40, -10, -1, 0, 1, 10, 40} {
			pixel := []float64{100 + dx, 200 + dy}

			world, err := wcs.PixelToWorld(pixel)
			if err != nil {
				t.Fatalf("PixelToWorld(%v): %v", pixel, err)
			}

			back, err := wcs.WorldToPixel(world)
			if err != nil {
				t.Fatalf("WorldToPixel(%v): %v", world, err)
			}

			if math.Abs(back[0]-pixel[0]) > 1e-6 || math.Abs(back[1]-pixel[1]) > 1e-6 {
				t.Errorf("pixel %v -> sky %v -> pixel %v", pixel, world, back)
			}
		}
	}
}

// Moving along a pixel axis has to move the sky in the direction the header
// says, which is what fixes the convention rather than leaving it to whichever
// half of the transform is read first.
//
// CDELT1 is negative here, the ordinary convention for a sky image: right
// ascension increases to the left, so a larger column is a smaller RA. CDELT2
// is positive, so a larger row is a larger declination.
func TestPixelToWorldFollowsTheHeaderOrientation(t *testing.T) {
	t.Parallel()

	wcs := tanWCS(t)

	centre, err := wcs.PixelToWorld([]float64{100, 200})
	if err != nil {
		t.Fatalf("PixelToWorld centre: %v", err)
	}

	if math.Abs(centre[0]-10) > 1e-9 || math.Abs(centre[1]-20) > 1e-9 {
		t.Fatalf("the reference pixel gave %v, want the reference value (10, 20)", centre)
	}

	east, err := wcs.PixelToWorld([]float64{110, 200})
	if err != nil {
		t.Fatalf("PixelToWorld +x: %v", err)
	}

	north, err := wcs.PixelToWorld([]float64{100, 210})
	if err != nil {
		t.Fatalf("PixelToWorld +y: %v", err)
	}

	// CDELT1 < 0: ten columns to the right is one degree of RA to the left.
	if got := east[0] - centre[0]; got > 0 {
		t.Errorf("with CDELT1 negative, +10 columns moved RA by %+.4f degrees; it must decrease", got)
	}

	// CDELT2 > 0: ten rows up is one degree further north.
	if got := north[1] - centre[1]; got < 0 {
		t.Errorf("with CDELT2 positive, +10 rows moved declination by %+.4f degrees; it must increase", got)
	}
}
