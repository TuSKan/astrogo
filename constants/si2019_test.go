package constants_test

import (
	"testing"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/unit"
)

func TestSI2019_ExactValues(t *testing.T) {
	testutil.AssertExact(t, "SpeedOfLight", constants.SI2019.SpeedOfLight.Value, 299_792_458.0)
	testutil.AssertExact(t, "PlanckConstant", constants.SI2019.PlanckConstant.Value, 6.626_070_15e-34)
	testutil.AssertExact(t, "BoltzmannConstant", constants.SI2019.BoltzmannConstant.Value, 1.380_649e-23)
}

func TestSI2019_AllExact(t *testing.T) {
	for _, c := range constants.SI2019.All() {
		if !c.Exact {
			t.Errorf("%s: Exact = false, want true", c.Symbol)
		}

		if c.Uncertainty != 0 {
			t.Errorf("%s: Uncertainty = %v, want 0", c.Symbol, c.Uncertainty)
		}
	}
}

func TestSI2019_Dimensions(t *testing.T) {
	c := constants.SI2019.SpeedOfLight
	if !c.Unit.Compatible(unit.Meter.Div(unit.Second)) {
		t.Errorf("SpeedOfLight.Unit not compatible with m/s")
	}

	if c.Unit.Compatible(unit.Meter) {
		t.Errorf("SpeedOfLight.Unit should not be compatible with a bare length")
	}

	wantAction := unit.Energy.Mul(unit.Time)
	if got := constants.SI2019.PlanckConstant.Unit.Dimension; got != wantAction {
		t.Errorf("PlanckConstant.Unit.Dimension = %+v, want %+v (J·s)", got, wantAction)
	}

	wantEntropy := unit.Energy.Div(unit.Temperature)
	if got := constants.SI2019.BoltzmannConstant.Unit.Dimension; got != wantEntropy {
		t.Errorf("BoltzmannConstant.Unit.Dimension = %+v, want %+v (J/K)", got, wantEntropy)
	}
}

func TestSI2019_Name(t *testing.T) {
	if got := constants.SI2019.Name(); got != "SI 2019" {
		t.Errorf("SI2019.Name() = %q, want %q", got, "SI 2019")
	}
}
