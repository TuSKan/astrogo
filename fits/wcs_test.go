package fits

import (
	"fmt"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestExtractWCS(t *testing.T) {
	h := NewHeader()
	h.Append(Card{Keyword: "NAXIS", Value: "2"})
	h.Append(Card{Keyword: "CRVAL1", Value: "10.0"})
	h.Append(Card{Keyword: "CRVAL2", Value: "20.0"})
	h.Append(Card{Keyword: "CRPIX1", Value: "100.0"})
	h.Append(Card{Keyword: "CRPIX2", Value: "200.0"})
	h.Append(Card{Keyword: "CDELT1", Value: "0.1"})
	h.Append(Card{Keyword: "CDELT2", Value: "0.1"})

	wcs, err := ExtractWCS(h)
	testutil.AssertNoError(t, err)
	if wcs.GetCRVAL()[0] != 10.0 || wcs.GetCRVAL()[1] != 20.0 {
		t.Errorf("expected CRVAL 10.0, 20.0, got %v", wcs.GetCRVAL())
	}

	h2 := NewHeader()
	_, err = ExtractWCS(h2)
	if err == nil {
		t.Errorf("expected error missing NAXIS")
	}

	h2.Append(Card{Keyword: "NAXIS", Value: "0"})
	_, err = ExtractWCS(h2)
	if err == nil {
		t.Errorf("expected error for NAXIS <= 0")
	}
}

func TestExtractWCS_CDMatrix(t *testing.T) {
	// Simulate a typical HST/JWST header using CDi_j (no PCi_j, no CDELT).
	h := NewHeader()
	h.Append(Card{Keyword: "NAXIS", Value: "2"})
	h.Append(Card{Keyword: "CTYPE1", Value: "'RA---TAN'"})
	h.Append(Card{Keyword: "CTYPE2", Value: "'DEC--TAN'"})
	h.Append(Card{Keyword: "CRVAL1", Value: "150.0"})
	h.Append(Card{Keyword: "CRVAL2", Value: "2.0"})
	h.Append(Card{Keyword: "CRPIX1", Value: "512.0"})
	h.Append(Card{Keyword: "CRPIX2", Value: "512.0"})

	// CD matrix: 0.05 arcsec/pixel with no rotation
	// CD1_1 = -0.05/3600, CD1_2 = 0, CD2_1 = 0, CD2_2 = 0.05/3600
	scale := 0.05 / 3600.0
	h.Append(Card{Keyword: "CD1_1", Value: fmt.Sprintf("%.15e", -scale)})
	h.Append(Card{Keyword: "CD1_2", Value: "0.0"})
	h.Append(Card{Keyword: "CD2_1", Value: "0.0"})
	h.Append(Card{Keyword: "CD2_2", Value: fmt.Sprintf("%.15e", scale)})

	wcs, err := ExtractWCS(h)
	testutil.AssertNoError(t, err)

	// At reference pixel → should return CRVAL exactly
	res, err := wcs.PixelToWorld([]float64{512.0, 512.0})
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "RA at CRPIX", res[0], 150.0, 1e-10)
	testutil.AssertNear(t, "DEC at CRPIX", res[1], 2.0, 1e-10)

	// CDELT should have been extracted from the CD matrix
	cdelt := wcs.GetCDELT()
	testutil.AssertNear(t, "CDELT1 magnitude", math.Abs(cdelt[0]), scale, 1e-20)
	testutil.AssertNear(t, "CDELT2 magnitude", math.Abs(cdelt[1]), scale, 1e-20)

	// CDELT1 should be negative (RA decreases with pixel X)
	if cdelt[0] >= 0 {
		t.Errorf("expected negative CDELT1 for RA axis, got %e", cdelt[0])
	}
}

func TestExtractWCS_CDMatrix_WithRotation(t *testing.T) {
	// CD matrix with 45° rotation and 1 deg/pixel scale
	h := NewHeader()
	h.Append(Card{Keyword: "NAXIS", Value: "2"})
	h.Append(Card{Keyword: "CTYPE1", Value: "'RA---TAN'"})
	h.Append(Card{Keyword: "CTYPE2", Value: "'DEC--TAN'"})
	h.Append(Card{Keyword: "CRVAL1", Value: "180.0"})
	h.Append(Card{Keyword: "CRVAL2", Value: "45.0"})
	h.Append(Card{Keyword: "CRPIX1", Value: "50.0"})
	h.Append(Card{Keyword: "CRPIX2", Value: "50.0"})

	// 45° rotation: cos(45°) ≈ 0.7071, sin(45°) ≈ 0.7071
	// CD1_1 = -scale*cos, CD1_2 = scale*sin, CD2_1 = -scale*sin, CD2_2 = -scale*cos
	c := math.Cos(math.Pi / 4.0)
	s := math.Sin(math.Pi / 4.0)
	scale := 0.001 // 3.6 arcsec/pixel
	h.Append(Card{Keyword: "CD1_1", Value: fmt.Sprintf("%.15e", -scale*c)})
	h.Append(Card{Keyword: "CD1_2", Value: fmt.Sprintf("%.15e", scale*s)})
	h.Append(Card{Keyword: "CD2_1", Value: fmt.Sprintf("%.15e", -scale*s)})
	h.Append(Card{Keyword: "CD2_2", Value: fmt.Sprintf("%.15e", -scale*c)})

	wcs, err := ExtractWCS(h)
	testutil.AssertNoError(t, err)

	// At reference pixel → CRVAL
	res, err := wcs.PixelToWorld([]float64{50.0, 50.0})
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "RA at CRPIX", res[0], 180.0, 1e-10)
	testutil.AssertNear(t, "DEC at CRPIX", res[1], 45.0, 1e-10)
}
