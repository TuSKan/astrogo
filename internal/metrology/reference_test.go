package metrology_test

import (
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/metrology"
)

// The shared-ancestry label is the most useful sentence this package can put
// in a report, and it has to be automatic.
//
// astrogo reaches IAU reductions through gofa, which is SOFA-derived. Astropy
// reaches them through ERFA, which is also SOFA-derived. Agreement between
// them says the two translations are faithful; it is not evidence that the
// model is right or that astrogo applies it in the right order. Nobody
// reading a table of accuracy figures should have to already know that.
func TestProvenanceMarksSharedAncestry(t *testing.T) {
	t.Parallel()

	shared := metrology.Reference{
		Kind:           metrology.KindSOFA,
		Name:           "gofa",
		Version:        "v1.19.1",
		Source:         "iauEpv00",
		SharedAncestor: "SOFA",
	}

	if shared.Independent() {
		t.Error("a reference sharing SOFA ancestry with astrogo reports as independent")
	}

	got := shared.Provenance()
	for _, want := range []string{"SOFA", "gofa", "v1.19.1", "iauEpv00", "consistency check", "not independent"} {
		if !strings.Contains(got, want) {
			t.Errorf("Provenance() = %q, missing %q", got, want)
		}
	}
}

func TestProvenanceOfAnIndependentReference(t *testing.T) {
	t.Parallel()

	independent := metrology.Reference{
		Kind:    metrology.KindHorizons,
		Name:    "JPL Horizons",
		Version: "DE441",
		Dataset: "de440.bsp",
		Source:  "https://ssd.jpl.nasa.gov/api/horizons.api",
	}

	if !independent.Independent() {
		t.Error("a reference with no shared ancestor reports as dependent")
	}

	got := independent.Provenance()

	if strings.Contains(got, "consistency check") {
		t.Errorf("an independent reference was labelled a consistency check: %q", got)
	}

	// The dataset is the field that keeps the claim checkable years later:
	// "validated against Horizons" ages into something nobody can repeat.
	if !strings.Contains(got, "de440.bsp") {
		t.Errorf("Provenance() = %q, missing the dataset it used", got)
	}
}

// A reference with nothing filled in must still render something rather than
// producing stray punctuation in a report.
func TestProvenanceOfAnEmptyReference(t *testing.T) {
	t.Parallel()

	got := metrology.Reference{Kind: metrology.KindInvariant, Name: "round trip"}.Provenance()
	if got != "INVARIANT round trip" {
		t.Errorf("Provenance() = %q, want %q", got, "INVARIANT round trip")
	}
}
