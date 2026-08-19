package crosssection_test

import (
	"context"
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/skybrightness/dataset/crosssection"
)

// The line absorbers must refuse rather than return nothing.
//
// This is the difference between a decision and an omission. A caller who asks
// for O2 or H2O absorption and receives an empty cross section cannot tell
// whether the model weighed them and found them negligible, or never had them
// at all. An error with a reason says which, and says it in code rather than
// in a document the caller may never read.
func TestLineAbsorbersRefuseWithAReason(t *testing.T) {
	t.Parallel()

	for _, species := range []string{"O2", "H2O"} {
		xs, err := crosssection.Species(context.Background(), species)
		if !errors.Is(err, crosssection.ErrLineAbsorber) {
			t.Errorf("%s: err = %v, want ErrLineAbsorber", species, err)
		}

		if len(xs.WavelengthNM) != 0 {
			t.Errorf("%s returned %d samples alongside its refusal", species, len(xs.WavelengthNM))
		}
	}
}

// An unknown species is a different failure from a known-but-unrepresentable
// one, and must not be mistaken for it.
func TestUnknownSpeciesIsNotALineAbsorber(t *testing.T) {
	t.Parallel()

	_, err := crosssection.Species(context.Background(), "NO2")
	if err == nil {
		t.Fatal("an unsourced species must fail")
	}

	if errors.Is(err, crosssection.ErrLineAbsorber) {
		t.Error("an unsourced species is not a line absorber; the two must stay distinguishable")
	}
}
