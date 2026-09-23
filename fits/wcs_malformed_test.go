package fits_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/fits"
)

// TestGetFloatReadsTheDExponent: the FITS standard lets a real keyword value
// use D as its exponent letter (NOST §5.2.4, FITS 4.0 §4.2.4), as
// Fortran-derived writers do for double precision. It was refused until #409.
func TestGetFloatReadsTheDExponent(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		value string
		want  float64
	}{
		{"1.2345678901234D-05", 1.2345678901234e-05},
		{"-2.5D+03", -2500},
		{"3.0D0", 3},
		{"1.0E-3", 1e-3},
		{"4.2", 4.2},
	} {
		h := wcsHeader(t, "X = "+c.value)

		got, err := h.GetFloat("X")
		if err != nil {
			t.Errorf("GetFloat(%q): %v", c.value, err)

			continue
		}

		if got != c.want {
			t.Errorf("GetFloat(%q) = %v, want %v", c.value, got, c.want)
		}
	}
}

// sipCards is a TAN-SIP header with a CD matrix and a second-order forward
// distortion, its reals written with exponent letter e.
func sipCards(e string) []string {
	return []string{
		"NAXIS = 2",
		"CTYPE1 = 'RA---TAN-SIP'",
		"CTYPE2 = 'DEC--TAN-SIP'",
		"CRVAL1 = 1.5" + e + "+02",
		"CRVAL2 = 2.0" + e + "+00",
		"CRPIX1 = 1.024" + e + "+03",
		"CRPIX2 = 1.024" + e + "+03",
		"CD1_1 = -2.8" + e + "-04",
		"CD1_2 = 0.0",
		"CD2_1 = 0.0",
		"CD2_2 = 2.8" + e + "-04",
		"A_ORDER = 2",
		"B_ORDER = 2",
		"A_2_0 = 1.2" + e + "-05",
		"A_0_2 = -3.4" + e + "-06",
		"B_1_1 = 5.6" + e + "-06",
	}
}

// TestExtractWCSReadsAHeaderWrittenWithDExponents: the same header written with
// D exponents throughout gives the same WCS as with E, SIP terms included.
// Before #409 every D-exponent keyword read as absent: this header came back
// with its reference point at (0, 0), unit scale and no distortion, and no
// error.
func TestExtractWCSReadsAHeaderWrittenWithDExponents(t *testing.T) {
	t.Parallel()

	withE, err := fits.ExtractWCS(wcsHeader(t, sipCards("E")...))
	if err != nil {
		t.Fatalf("E exponents: %v", err)
	}

	withD, err := fits.ExtractWCS(wcsHeader(t, sipCards("D")...))
	if err != nil {
		t.Fatalf("D exponents: %v", err)
	}

	// Precondition: the SIP terms move the corner, so a D-exponent reader that
	// dropped them could not pass by agreeing with an E reader that did too.
	var noSIP []string

	for _, c := range sipCards("E") {
		if !strings.HasPrefix(c, "A_") && !strings.HasPrefix(c, "B_") {
			noSIP = append(noSIP, c)
		}
	}

	undistorted, err := fits.ExtractWCS(wcsHeader(t, noSIP...))
	if err != nil {
		t.Fatalf("no SIP: %v", err)
	}

	corner := []float64{1, 1}

	e, err := withE.PixelToWorld(corner)
	if err != nil {
		t.Fatalf("E: PixelToWorld: %v", err)
	}

	d, err := withD.PixelToWorld(corner)
	if err != nil {
		t.Fatalf("D: PixelToWorld: %v", err)
	}

	u, err := undistorted.PixelToWorld(corner)
	if err != nil {
		t.Fatalf("no SIP: PixelToWorld: %v", err)
	}

	if math.Abs(e[0]-u[0])*3600 < 0.1 && math.Abs(e[1]-u[1])*3600 < 0.1 {
		t.Fatalf("precondition: the SIP terms move the corner by under 0.1″ (%v vs %v)", e, u)
	}

	for i := range e {
		if d[i] != e[i] {
			t.Errorf("axis %d: %v with D exponents, %v with E", i+1, d[i], e[i])
		}
	}
}

// TestExtractWCSRefusesAMalformedKeyword: a WCS keyword the header states but
// that does not parse is an error, not its default. Until #409 each of these
// was taken for absent — CRVAL and CRPIX read 0, CDELT 1, PC the identity, a
// SIP or TPV coefficient zero — and a malformed NAXIS was reported as missing.
func TestExtractWCSRefusesAMalformedKeyword(t *testing.T) {
	t.Parallel()

	tpv := []string{
		"NAXIS = 2",
		"CTYPE1 = 'RA---TPV'",
		"CTYPE2 = 'DEC--TPV'",
		"CRVAL1 = 150.0",
		"CRVAL2 = 2.0",
		"CRPIX1 = 1.0",
		"CRPIX2 = 1.0",
		"CDELT1 = -0.001",
		"CDELT2 = 0.001",
		"PV1_1 = 1.0",
	}

	pc := []string{
		"NAXIS = 2",
		"CTYPE1 = 'RA---TAN'",
		"CTYPE2 = 'DEC--TAN'",
		"CRVAL1 = 150.0",
		"CRVAL2 = 2.0",
		"CRPIX1 = 1.0",
		"CRPIX2 = 1.0",
		"CDELT1 = -0.001",
		"CDELT2 = 0.001",
		"PC1_2 = 0.0",
	}

	for _, c := range []struct {
		base []string
		key  string
	}{
		{sipCards("E"), "CRVAL1"},
		{sipCards("E"), "CRPIX2"},
		{sipCards("E"), "CD1_1"},
		{sipCards("E"), "A_2_0"},
		{sipCards("E"), "A_ORDER"},
		{pc, "CDELT1"},
		{pc, "PC1_2"},
		{tpv, "PV1_1"},
		{pc, "NAXIS"},
	} {
		cards := make([]string, 0, len(c.base))

		for _, card := range c.base {
			if k, _, _ := strings.Cut(card, "="); strings.TrimSpace(k) == c.key {
				card = c.key + " = 1.5Q-03"
			}

			cards = append(cards, card)
		}

		_, err := fits.ExtractWCS(wcsHeader(t, cards...))
		if !errors.Is(err, fits.ErrWCSMalformedKeyword) {
			t.Errorf("%s malformed: err %v, want ErrWCSMalformedKeyword", c.key, err)
		}

		if errors.Is(err, fits.ErrWCSMissingNAXIS) {
			t.Errorf("%s malformed: reported as a missing NAXIS", c.key)
		}
	}
}
