package luminosity_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/magnitude"
	"github.com/TuSKan/astrogo/skybrightness/dataset/luminosity"
)

// The parser reads CVRL's two-column form and rejects what it cannot use.
func TestParseReadsTheCVRLForm(t *testing.T) {
	t.Parallel()

	// Real CVRL rows, including their leading-space formatting and the blank
	// trailing line their files carry.
	band, err := luminosity.Parse(strings.NewReader(
		"380, 0.0005890000\n" +
			"381, 0.0006650000\n" +
			"382, 0.0007520000\n" +
			"\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(band.WavelengthNM) != 3 {
		t.Fatalf("parsed %d rows, want 3 — the blank trailing line must be skipped, not "+
			"counted", len(band.WavelengthNM))
	}

	if got := float64(band.WavelengthNM[0]); got != 380 {
		t.Errorf("first wavelength is %g, want 380", got)
	}

	if got := band.Response[2]; got != 0.0007520000 {
		t.Errorf("third response is %g, want 0.000752", got)
	}

	// The detector convention is not a free choice: V(lambda) weights radiant
	// power, so integrating it as though it counted photons tilts the answer
	// across the band by a factor of wavelength.
	if band.Detector != magnitude.EnergyIntegrating {
		t.Errorf("detector is %v, want EnergyIntegrating", band.Detector)
	}
}

// A curve whose wavelengths do not ascend is refused.
//
// Every consumer interpolates, and a repeated or reversed sample makes that
// silently wrong rather than visibly broken — the curve still integrates and
// still returns a plausible number.
func TestParseRefusesNonAscendingWavelengths(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name string
		body string
	}{
		{"repeated", "500, 0.9\n500, 0.8\n501, 0.7\n"},
		{"reversed", "500, 0.9\n499, 0.8\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if _, err := luminosity.Parse(strings.NewReader(c.body)); !errors.Is(err, luminosity.ErrCurve) {
				t.Errorf("got %v, want ErrCurve", err)
			}
		})
	}
}

// Rows that are not data are skipped, but a file with nothing usable is an
// error rather than an empty curve.
func TestParseSkipsJunkButRefusesAnEmptyCurve(t *testing.T) {
	t.Parallel()

	// A header line, a malformed row and a negative response, around two
	// real ones.
	band, err := luminosity.Parse(strings.NewReader(
		"wavelength, value\n500, 0.9\nnonsense\n501, -0.2\n502, 0.7\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(band.WavelengthNM) != 2 {
		t.Errorf("parsed %d rows, want 2 — a negative response is not a measurement",
			len(band.WavelengthNM))
	}

	if _, err := luminosity.Parse(strings.NewReader("header only\n\n")); !errors.Is(err, luminosity.ErrCurve) {
		t.Errorf("a file with no usable rows returned %v, want ErrCurve", err)
	}
}

// Each vision names itself and carries its own efficacy.
//
// The efficacies are definitional and the pairing is the thing worth
// pinning: handing the photopic 683 to the scotopic curve is the one way to
// hold this wrong, and it is wrong by a factor of two and a half.
func TestVisionCarriesItsOwnEfficacy(t *testing.T) {
	t.Parallel()

	if got := luminosity.Photopic.Efficacy(); got != luminosity.PhotopicEfficacy {
		t.Errorf("photopic efficacy is %g, want %g", got, luminosity.PhotopicEfficacy)
	}

	if got := luminosity.Scotopic.Efficacy(); got != luminosity.ScotopicEfficacy {
		t.Errorf("scotopic efficacy is %g, want %g", got, luminosity.ScotopicEfficacy)
	}

	if luminosity.ScotopicEfficacy <= luminosity.PhotopicEfficacy {
		t.Error("the scotopic maximum must exceed the photopic one: the dark-adapted eye " +
			"is the more sensitive of the two")
	}

	for _, c := range []struct {
		v    luminosity.Vision
		want string
	}{
		{luminosity.Photopic, "photopic"},
		{luminosity.Scotopic, "scotopic"},
		{luminosity.Vision(7), "Vision(unknown)"},
	} {
		if got := c.v.String(); got != c.want {
			t.Errorf("Vision(%d).String() = %q, want %q", int(c.v), got, c.want)
		}
	}
}
