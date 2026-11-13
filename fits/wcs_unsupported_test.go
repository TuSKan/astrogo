package fits_test

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/fits"
)

// TestExtractWCSRefusesAProjectionItDoesNotImplement is #1125. A celestial
// axis whose projection the package does not implement was taken for a linear
// one: no error, and a frame's center 7″ off on a 1″-per-pixel frame. ZEA, CAR
// and MOL are the standard projections of real survey maps.
func TestExtractWCSRefusesAProjectionItDoesNotImplement(t *testing.T) {
	t.Parallel()

	header := func(ctype1, ctype2 string) *fits.Header {
		return wcsHeader(t,
			"NAXIS = 2",
			"CTYPE1 = '"+ctype1+"'",
			"CTYPE2 = '"+ctype2+"'",
			"CRVAL1 = 150.0",
			"CRVAL2 = 2.0",
			"CRPIX1 = 1.0",
			"CRPIX2 = 1.0",
			"CDELT1 = -0.001",
			"CDELT2 = 0.001",
		)
	}

	for _, c := range [][2]string{
		{"RA---ZEA", "DEC--ZEA"},
		{"RA---CAR", "DEC--CAR"},
		{"GLON-MOL", "GLAT-MOL"},
		{"ELON-ZPN", "ELAT-ZPN"},
		{"RA---ZZZ", "DEC--ZZZ"},
		{"RA---ZEA-SIP", "DEC--ZEA-SIP"},
	} {
		if _, err := fits.ExtractWCS(header(c[0], c[1])); !errors.Is(err, fits.ErrWCSUnsupported) {
			t.Errorf("%s / %s: %v, want %v", c[0], c[1], err, fits.ErrWCSUnsupported)
		}
	}

	// Axes that name no celestial projection are linear, as before, and a
	// projection the package does implement is still read.
	for _, c := range [][2]string{
		{"LINEAR", "LINEAR"},
		{"FREQ", "STOKES"},
		{"", ""},
		{"RA---TAN", "DEC--TAN"},
		{"GLON-AIT", "GLAT-AIT"},
	} {
		if _, err := fits.ExtractWCS(header(c[0], c[1])); err != nil {
			t.Errorf("%q / %q: %v, want it read", c[0], c[1], err)
		}
	}
}

// TestSetCTYPEKeepsTheWCSItRefuses: a refused CTYPE leaves the WCS as it was,
// so a caller that ignores nothing still holds a usable projection.
func TestSetCTYPEKeepsTheWCSItRefuses(t *testing.T) {
	t.Parallel()

	w := fits.NewWCS(2)
	if err := w.SetCTYPE([]string{"RA---TAN", "DEC--TAN"}); err != nil {
		t.Fatal(err)
	}

	if err := w.SetCTYPE([]string{"RA---ZEA", "DEC--ZEA"}); !errors.Is(err, fits.ErrWCSUnsupported) {
		t.Fatalf("SetCTYPE(ZEA): %v, want %v", err, fits.ErrWCSUnsupported)
	}

	if got := w.CTYPE(); got[0] != "RA---TAN" || got[1] != "DEC--TAN" {
		t.Errorf("after a refused CTYPE the WCS holds %q, want the TAN it had", got)
	}
}
