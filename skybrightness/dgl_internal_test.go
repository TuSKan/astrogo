package skybrightness

import (
	"strings"
	"testing"
)

// TestKawaraTableIsTranscribedCorrectly checks dglCoefficients against Kawara
// et al. (2017) Table 2, row by row, uncertainties included (#922).
//
// The values below were read from the paper's own LaTeX source (arXiv
// 1701.00885). A wrong digit in this table changes every diffuse galactic
// light the package predicts, and the sky total by only a few per cent, which
// no end-to-end test could tell from the model's real error. The provenance
// once quoted "10 to 50 per cent" for the slope's uncertainty, which this table
// does not say: transcribing the uncertainty columns too is what keeps a
// figure quoted about the table answerable to it.
func TestKawaraTableIsTranscribedCorrectly(t *testing.T) {
	t.Parallel()

	want := [8]dglCoefficient{
		// Effective wavelength (nm), nu*b, its sigma, c (1e-5), its sigma.
		{225, 3.0, 1.4, 0.1, 0.4},
		{274, 3.9, 0.3, 0.3, 0.1},
		{319, 6.1, 0.4, 0.7, 0.1},
		{369, 8.5, 0.5, 1.1, 0.2},
		{418, 13.6, 0.5, 2.4, 0.3},
		{472, 17.5, 0.6, 3.3, 0.3},
		{550, 20.1, 0.6, 4.4, 0.4},
		{648, 21.0, 0.9, 4.5, 0.7},
	}

	for i, w := range want {
		if got := dglCoefficients[i]; got != w {
			t.Errorf("row %d = %+v, want %+v", i, got, w)
		}
	}
}

// TestDGLSlopeUncertaintyMatchesTheProvenance holds the figures the provenance
// quotes to the table it quotes them from: 3 to 8 per cent from 274 to 648 nm,
// and 47 at 225 nm, rounded as the text rounds them.
func TestDGLSlopeUncertaintyMatchesTheProvenance(t *testing.T) {
	t.Parallel()

	lo, hi := 1.0, 0.0

	for _, row := range dglCoefficients[1:] {
		rel := row.SigmaNuB / row.NuB
		lo, hi = min(lo, rel), max(hi, rel)
	}

	if lo < 0.025 || hi >= 0.085 {
		t.Errorf("slope uncertainty spans %.1f-%.1f per cent from 274 to 648 nm, "+
			"but the provenance says 3-8", 100*lo, 100*hi)
	}

	if rel := dglCoefficients[0].SigmaNuB / dglCoefficients[0].NuB; rel < 0.465 || rel >= 0.475 {
		t.Errorf("slope uncertainty at 225 nm is %.1f per cent, but the provenance says 47", 100*rel)
	}

	text := (&DiffuseGalacticLight{}).Provenance().ExpectedAccuracy
	for _, figure := range []string{"3-8 per cent from 274 to 648 nm", "47 per cent at 225 nm"} {
		if !strings.Contains(text, figure) {
			t.Errorf("the provenance no longer quotes %q, which this test checks against the table: %q", figure, text)
		}
	}
}
