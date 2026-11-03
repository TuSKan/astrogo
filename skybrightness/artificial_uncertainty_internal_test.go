package skybrightness

import (
	"fmt"
	"strings"
	"testing"
)

// TestKocifajFiguresMatchTheProvenance holds the constants the component
// computes with to the figures its provenance quotes from Kocifaj, Bara &
// Falchi (2022) Sec. 3, so neither can move alone (#933).
func TestKocifajFiguresMatchTheProvenance(t *testing.T) {
	t.Parallel()

	p := (&ArtificialSkyglow{}).Provenance()

	// 25% is the top of the 20-25% and 15-25% ranges the text quotes.
	if kocifajModelDiscrepancy != 0.25 {
		t.Errorf("the model discrepancy is %v; the paper's ranges top out at 0.25", kocifajModelDiscrepancy)
	}

	for _, figure := range []string{
		"20% aerosol-free at 450 nm",
		"20-25% at aerosol optical depths of 0.23 and 0.57",
		"15-25% across the 550 nm set",
		fmt.Sprintf("carries %.0f%%", 100*kocifajModelDiscrepancy),
	} {
		if !strings.Contains(p.ExpectedAccuracy, figure) {
			t.Errorf("ExpectedAccuracy no longer quotes %q: %q", figure, p.ExpectedAccuracy)
		}
	}

	for _, figure := range []string{
		fmt.Sprintf("%.1f to %.1f km", kocifajNearestKM, kocifajFarthestKM),
		fmt.Sprintf("up to %.2f", kocifajLargestAOD),
	} {
		if !strings.Contains(p.ValidityDomain, figure) {
			t.Errorf("ValidityDomain does not state the bound the code flags at, %q: %q", figure, p.ValidityDomain)
		}
	}
}
