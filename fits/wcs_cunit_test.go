package fits_test

import (
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/fits"
)

// wcsHeader builds a minimal header from CARD text, one card per line.
func wcsHeader(t *testing.T, cards ...string) *fits.Header {
	t.Helper()

	h := fits.NewHeader()

	for _, c := range cards {
		k, v, found := strings.Cut(c, "=")
		if !found {
			t.Fatalf("malformed test card %q", c)
		}

		h.Append(fits.Card{
			Keyword: strings.TrimSpace(k),
			Value:   strings.TrimSpace(v),
		})
	}

	return h
}

// TestCUnitSurvivesTheHeader covers the gap #178 named from the other side.
//
// The issue's complaint is that PixelToWorld returns a bare []float64 whose
// unit a caller cannot determine. For celestial axes the answer is fixed —
// WCS Paper II requires degrees — but for a spectral or time axis the value is
// CRVAL plus a linear offset in whatever CUNITi declares, and astrogo did not
// read CUNIT at all. The number was correct and its unit was unreachable
// through the API, which is a worse combination than either alone: a caller
// gets a plausible float and has to go back to the header card to learn
// whether it is metres or Angstrom.
//
// This does not decide the typed-API question. It makes the input that
// question needs visible, which is a precondition for answering it either way.
func TestCUnitSurvivesTheHeader(t *testing.T) {
	t.Parallel()

	h := wcsHeader(t,
		"NAXIS = 3",
		"CTYPE1 = 'RA---TAN'",
		"CTYPE2 = 'DEC--TAN'",
		"CTYPE3 = 'WAVE'",
		"CRVAL1 = 150.0",
		"CRVAL2 = 2.0",
		"CRVAL3 = 5.0e-7",
		"CRPIX1 = 1.0",
		"CRPIX2 = 1.0",
		"CRPIX3 = 1.0",
		"CDELT1 = -0.001",
		"CDELT2 = 0.001",
		"CDELT3 = 1.0e-10",
		"CUNIT1 = 'deg'",
		"CUNIT2 = 'deg'",
		"CUNIT3 = 'm'",
	)

	w, err := fits.ExtractWCS(h)
	if err != nil {
		t.Fatalf("ExtractWCS: %v", err)
	}

	got := w.CUnit()

	want := []string{"deg", "deg", "m"}
	if len(got) != len(want) {
		t.Fatalf("CUnit() has %d entries, want %d", len(got), len(want))
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("CUnit()[%d] = %q, want %q.\n"+
				"  The value PixelToWorld returns for axis %d is in this unit and no other "+
				"part of the API says which it is (#178).", i, got[i], want[i], i+1)
		}
	}
}

// TestCUnitIsEmptyWhenTheHeaderIsSilent pins the absence case.
//
// CUNIT is optional and often missing. An absent card is recorded as an empty
// string rather than defaulted to "deg", because "the header did not say" and
// "the header said degrees" are different facts, and only the caller knows
// whether the difference matters to them. Defaulting would turn a question
// they could still ask into an answer they cannot check — the error-versus-
// absence rule this repository applies elsewhere.
func TestCUnitIsEmptyWhenTheHeaderIsSilent(t *testing.T) {
	t.Parallel()

	h := wcsHeader(t,
		"NAXIS = 2",
		"CTYPE1 = 'RA---TAN'",
		"CTYPE2 = 'DEC--TAN'",
		"CRVAL1 = 150.0",
		"CRVAL2 = 2.0",
		"CRPIX1 = 1.0",
		"CRPIX2 = 1.0",
		"CDELT1 = -0.001",
		"CDELT2 = 0.001",
	)

	w, err := fits.ExtractWCS(h)
	if err != nil {
		t.Fatalf("ExtractWCS: %v", err)
	}

	for i, u := range w.CUnit() {
		if u != "" {
			t.Errorf("CUnit()[%d] = %q for a header with no CUNIT card, want \"\".\n"+
				"  Inventing a unit the header never stated would make an assumption "+
				"indistinguishable from a fact.", i, u)
		}
	}
}

// TestCUnitDoesNotAliasTheWCS is the same check #220 applied to every other
// accessor, applied to the new one before it can go wrong.
//
// #220 found the getters returning the internal slice and the setters keeping
// the caller's, so reading CRVAL to inspect it let you move an image on the
// sky at a distance. A new accessor pair that repeated the mistake would be a
// regression of a fixed bug.
func TestCUnitDoesNotAliasTheWCS(t *testing.T) {
	t.Parallel()

	w := fits.NewWCS(2)

	if err := w.SetCUnit([]string{"deg", "deg"}); err != nil {
		t.Fatalf("SetCUnit: %v", err)
	}

	// Mutating what the getter returned must not reach the WCS.
	got := w.CUnit()
	got[0] = "arcsec"

	if again := w.CUnit(); again[0] != "deg" {
		t.Errorf("CUnit()[0] became %q after a caller mutated an earlier result.\n"+
			"  The getter must copy; see #220.", again[0])
	}

	// Nor must holding the slice that was passed in.
	held := []string{"deg", "deg"}
	if err := w.SetCUnit(held); err != nil {
		t.Fatalf("SetCUnit: %v", err)
	}

	held[1] = "Hz"

	if again := w.CUnit(); again[1] != "deg" {
		t.Errorf("CUnit()[1] became %q after the caller mutated the slice they set.\n"+
			"  The setter must copy; see #220.", again[1])
	}
}

// TestSetCUnitRefusesTheWrongLength matches the invariant #220 gave the other
// setters with a length rule: a short or long slice is a disagreement about
// axis count, and accepting it silently is what that change stopped doing.
func TestSetCUnitRefusesTheWrongLength(t *testing.T) {
	t.Parallel()

	w := fits.NewWCS(3)

	for _, tc := range [][]string{{}, {"deg"}, {"deg", "deg"}, {"deg", "deg", "deg", "m"}} {
		if err := w.SetCUnit(tc); err == nil {
			t.Errorf("SetCUnit(%d entries) on a 3-axis WCS returned no error.", len(tc))
		}
	}

	if err := w.SetCUnit([]string{"deg", "deg", "m"}); err != nil {
		t.Errorf("SetCUnit with the right length: %v", err)
	}
}
