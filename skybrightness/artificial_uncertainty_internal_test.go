package skybrightness

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/unit"
)

// TestArtificialSourceFailsOnStateItCannotEvaluate: addTo's two error
// returns are out of reach through the component, which clamps the
// asymmetry and derives t from positive optical depths. A source whose state
// is invalid must still fail, adding nothing and reporting itself unlit,
// rather than put a half-evaluated band into the radiance or the budget.
func TestArtificialSourceFailsOnStateItCannotEvaluate(t *testing.T) {
	t.Parallel()

	source := func(g, opticalT float64) *artificialSource {
		return &artificialSource{
			rayleigh:  []unit.OpticalDepth{0.1},
			aerosol:   []unit.OpticalDepth{0.05},
			asymmetry: []float64{g},
			optical:   []float64{opticalT},
			arriving:  []float64{1e-3},
			relative:  0.1,
			known:     true,
		}
	}

	for _, c := range []struct {
		name string
		src  *artificialSource
		want error
	}{
		{"an asymmetry outside (-1, 1)", source(1.5, 0.2), atmosphere.ErrAsymmetry},
		{"a t that is not positive", source(0.5, 0), ErrOpticalParameter},
	} {
		dst, weighted := []float64{0}, []float64{0}

		lit, err := c.src.addTo(dst, weighted, angle.Deg(30), 2, 1.2)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}

		if lit || dst[0] != 0 || weighted[0] != 0 {
			t.Errorf("%s: lit = %v, added %g radiance and %g weighted", c.name, lit, dst[0], weighted[0])
		}
	}
}

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
