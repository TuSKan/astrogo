package constants_test

import (
	"testing"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/unit"
)

func TestWGS84_SemiMajorAxis(t *testing.T) {
	testutil.AssertExact(t, "WGS84 SemiMajorAxis", constants.WGS84.SemiMajorAxis.Value, 6_378_137.0)
}

func TestWGS84_InverseFlattening(t *testing.T) {
	testutil.AssertExact(t, "WGS84 InverseFlattening", constants.WGS84.InverseFlattening.Value, 298.257_223_563)
}

func TestWGS84_PolarRadius(t *testing.T) {
	a := constants.WGS84.SemiMajorAxis.Value
	b := a * (1 - constants.Derived.WGS84Flattening.Value)

	if b >= a {
		t.Errorf("polar radius %v should be less than semi-major axis %v", b, a)
	}

	if b < 6.356e6 || b > 6.357e6 {
		t.Errorf("polar radius %v outside plausible band [6.356e6, 6.357e6]", b)
	}
}

func TestWGS84_Units(t *testing.T) {
	if constants.WGS84.SemiMajorAxis.Unit != unit.Meter {
		t.Errorf("SemiMajorAxis.Unit = %v, want unit.Meter", constants.WGS84.SemiMajorAxis.Unit)
	}

	if constants.WGS84.InverseFlattening.Unit.Dimension != unit.Dimensionless {
		t.Errorf("InverseFlattening.Unit.Dimension = %+v, want Dimensionless", constants.WGS84.InverseFlattening.Unit.Dimension)
	}
}

func TestWGS84_BothExact(t *testing.T) {
	for _, c := range constants.WGS84.All() {
		if !c.Exact {
			t.Errorf("%s: Exact = false, want true", c.Symbol)
		}

		if c.Uncertainty != 0 {
			t.Errorf("%s: Uncertainty = %v, want 0", c.Symbol, c.Uncertainty)
		}
	}
}

func TestWGS84_Name(t *testing.T) {
	if got := constants.WGS84.Name(); got == "" {
		t.Errorf("WGS84.Name() is empty")
	}
}
