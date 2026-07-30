package constants_test

import (
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/unit"
)

func TestDerived_JulianDaySeconds(t *testing.T) {
	c := constants.Derived.JulianDaySeconds
	testutil.AssertExact(t, "JulianDaySeconds", c.Value, 86400.0)

	if c.Unit != unit.Second {
		t.Errorf("JulianDaySeconds.Unit = %v, want unit.Second", c.Unit)
	}

	// Ties this package's notion of a day to unit's.
	testutil.AssertExact(t, "JulianDaySeconds vs unit.Day.ScaleFactor", c.Value, unit.Day.ScaleFactor)
}

func TestDerived_AngleFactors_Inverse(t *testing.T) {
	product := constants.Derived.RadiansPerDegree.Value * constants.Derived.DegreesPerRadian.Value
	testutil.AssertRelNear(t, "RadiansPerDegree * DegreesPerRadian", product, 1.0, 1e-15)
}

func TestDerived_AngleFactors_Values(t *testing.T) {
	testutil.AssertRelNear(t, "RadiansPerDegree", constants.Derived.RadiansPerDegree.Value, math.Pi/180, 1e-15)
	testutil.AssertRelNear(t, "DegreesPerRadian", constants.Derived.DegreesPerRadian.Value, 180/math.Pi, 1e-15)

	arcsecPerRad := constants.Derived.ArcSecondsPerRadian.Value
	if arcsecPerRad < 206264.0 || arcsecPerRad > 206265.5 {
		t.Errorf("ArcSecondsPerRadian = %v, outside plausible band [206264.0, 206265.5]", arcsecPerRad)
	}
}

// TestDerived_ArcSecondsPerRadian_Relation intentionally uses a relative
// tolerance, not exact equality: Value is now a runtime float64 field, so
// 3600 * DegreesPerRadian.Value is a runtime multiply of an
// already-rounded operand and can differ from the directly-computed
// ArcSecondsPerRadian by up to 1 ULP.
func TestDerived_ArcSecondsPerRadian_Relation(t *testing.T) {
	want := 3600 * constants.Derived.DegreesPerRadian.Value
	testutil.AssertRelNear(t, "ArcSecondsPerRadian vs 3600*DegreesPerRadian",
		constants.Derived.ArcSecondsPerRadian.Value, want, 1e-15)
}

// TestDerived_AgreesWithUnitScaleFactors holds exactly (not just within
// tolerance) because both RadiansPerDegree and unit.Degree.ScaleFactor
// are initialized from the identical constant expression math.Pi/180 —
// if this ever fails, a local duplicate of math.Pi crept back in.
func TestDerived_AgreesWithUnitScaleFactors(t *testing.T) {
	testutil.AssertExact(t, "RadiansPerDegree vs unit.Degree.ScaleFactor",
		constants.Derived.RadiansPerDegree.Value, unit.Degree.ScaleFactor)

	testutil.AssertRelNear(t, "ArcSecondsPerRadian vs 1/unit.Arcsecond.ScaleFactor",
		constants.Derived.ArcSecondsPerRadian.Value, 1/unit.Arcsecond.ScaleFactor, 1e-15)
}

func TestDerived_WGS84Flattening(t *testing.T) {
	f := constants.Derived.WGS84Flattening

	if f.Value < 0.003352 || f.Value > 0.003353 {
		t.Errorf("WGS84Flattening = %v, outside plausible band [0.003352, 0.003353]", f.Value)
	}

	testutil.AssertExact(t, "WGS84Flattening vs 1/WGS84.InverseFlattening",
		f.Value, 1.0/constants.WGS84.InverseFlattening.Value)

	if !strings.Contains(f.Reference, "WGS84.InverseFlattening") {
		t.Errorf("WGS84Flattening.Reference = %q, want it to mention WGS84.InverseFlattening", f.Reference)
	}
}

func TestDerived_FullCircleAndRightAngle(t *testing.T) {
	testutil.AssertRelNear(t, "360 degrees in radians", 360*constants.Derived.RadiansPerDegree.Value, 2*math.Pi, 1e-15)
	testutil.AssertRelNear(t, "90 degrees in radians", 90*constants.Derived.RadiansPerDegree.Value, math.Pi/2, 1e-15)
}

func TestDerived_AllExact(t *testing.T) {
	for _, c := range constants.Derived.All() {
		if !c.Exact {
			t.Errorf("%s: Exact = false, want true", c.Symbol)
		}

		if c.Uncertainty != 0 {
			t.Errorf("%s: Uncertainty = %v, want 0", c.Symbol, c.Uncertainty)
		}
	}
}

func TestDerived_Name(t *testing.T) {
	if got := constants.Derived.Name(); got != "derived" {
		t.Errorf("Derived.Name() = %q, want %q", got, "derived")
	}
}
