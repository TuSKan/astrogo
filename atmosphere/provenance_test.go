package atmosphere_test

import (
	"testing"

	"github.com/TuSKan/astrogo/atmosphere"
)

// Every fidelity level names itself, and an unrecognised one says so rather
// than passing for a real level.
//
// # Why a stringer is worth a test
//
// Because this string is not decoration. Fidelity is how a caller learns
// whether a number came from a measurement, a model, a regional prior or a
// test fixture, and it travels with results into logs and provenance records.
// A level that rendered as another level's name — or that rendered as an
// empty string — would misdescribe the data at exactly the moment somebody
// was trying to establish where it came from.
//
// The default branch matters most: a Fidelity read from a file, or from a
// future version that added a level, must not silently print as "Measured".
func TestFidelityNamesEveryLevel(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		fidelity atmosphere.Fidelity
		want     string
	}{
		{atmosphere.FidelityMeasured, "Measured"},
		{atmosphere.FidelityModelPropagated, "ModelPropagated"},
		{atmosphere.FidelityPrior, "Prior"},
		{atmosphere.FidelitySynthetic, "Synthetic"},
	} {
		if got := c.fidelity.String(); got != c.want {
			t.Errorf("Fidelity(%d).String() = %q, want %q", c.fidelity, got, c.want)
		}
	}

	// The zero value is Measured, which is worth stating explicitly: it means
	// an unset Fidelity claims the strongest provenance there is, so a
	// SourceRef that forgot to set one overclaims rather than underclaims.
	var unset atmosphere.Fidelity
	if unset != atmosphere.FidelityMeasured {
		t.Errorf("the zero Fidelity is %v, not FidelityMeasured", unset)
	}

	for _, unknown := range []atmosphere.Fidelity{4, 17, 255} {
		if got := unknown.String(); got != "Fidelity(unknown)" {
			t.Errorf("Fidelity(%d).String() = %q, want %q — an unrecognised level must not "+
				"borrow a real level's name", unknown, got, "Fidelity(unknown)")
		}
	}
}
