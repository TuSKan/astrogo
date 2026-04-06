package unit_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/unit"
)

func TestDimensionAlgebra(t *testing.T) {
	// L * L = L^2
	areaDim := unit.Length.Mul(unit.Length)
	testutil.AssertEqual(t, "Length*Length == Area", areaDim, unit.Area)

	// L / T = Velocity
	velDim := unit.Length.Div(unit.Time)
	testutil.AssertEqual(t, "Length/Time == Velocity", velDim, unit.Velocity)

	// L^3 = Volume
	volDim := unit.Length.PowInt(3)
	testutil.AssertEqual(t, "Length^3 == Volume", volDim, unit.Volume)

	// Inquality
	if unit.Length.Equals(unit.Mass) {
		t.Error("Length should not equal Mass")
	}
}

func TestUnitCompatibility(t *testing.T) {
	if !unit.Meter.Compatible(unit.Kilometer) {
		t.Error("Meter should be compatible with Kilometer")
	}
	if unit.Meter.Compatible(unit.Second) {
		t.Error("Meter should not be compatible with Second")
	}
}

func TestConversionFactors(t *testing.T) {
	cases := []struct {
		from   unit.Unit
		to     unit.Unit
		factor float64
	}{
		{unit.Kilometer, unit.Meter, 1000},
		{unit.Meter, unit.Kilometer, 0.001},
		{unit.Hour, unit.Second, 3600},
		{unit.Second, unit.Minute, 1.0 / 60.0},
		{unit.Degree, unit.Radian, math.Pi / 180.0},
		{unit.Arcminute, unit.Degree, 1.0 / 60.0},
	}

	for i, c := range cases {
		factor, err := c.from.ConversionFactor(c.to)
		testutil.AssertNoError(t, err)
		testutil.AssertNear(t, testutil.CaseLabel(i, c.from.Symbol+" to "+c.to.Symbol), factor, c.factor, 1e-15)
	}
}

func TestIncompatibleConversion(t *testing.T) {
	_, err := unit.Meter.ConversionFactor(unit.Second)
	testutil.AssertError(t, err)
	testutil.AssertErrorContains(t, err, "incompatible unit conversion")
}

func TestDerivedUnits(t *testing.T) {
	// m/s^2 (Acceleration)
	accel := unit.Meter.Div(unit.Second.PowInt(2))
	testutil.AssertEqual(t, "m/s^2 dimension", accel.Dimension, unit.Accel)
	testutil.AssertNear(t, "m/s^2 scale", accel.ScaleFactor, 1.0, 1e-15)

	// km/h (Velocity)
	kmph := unit.Kilometer.Div(unit.Hour)
	testutil.AssertEqual(t, "km/h dimension", kmph.Dimension, unit.Velocity)
	// 1000 m / 3600 s = 1/3.6 m/s
	testutil.AssertNear(t, "km/h scale", kmph.ScaleFactor, 1000.0/3600.0, 1e-15)
}

func TestUnitAlgebra(t *testing.T) {
	m2 := unit.Meter.Mul(unit.Meter)
	testutil.AssertEqual(t, "m*m dimension", m2.Dimension, unit.Area)
	testutil.AssertNear(t, "m*m scale", m2.ScaleFactor, 1.0, 1e-15)

	newton := unit.Kilogram.Mul(unit.Meter).Div(unit.Second.PowInt(2))
	testutil.AssertEqual(t, "Newton dimension", newton.Dimension, unit.Force)
}

func TestAstronomicalUnits(t *testing.T) {
	// 1 AU in km
	factor, err := unit.AstronomicalUnit.ConversionFactor(unit.Kilometer)
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "1 AU in km", factor, 1.495978707e8, 1e3) // ~1.496e8 km

	// 1 pc in AU
	factorPcAU, err := unit.Parsec.ConversionFactor(unit.AstronomicalUnit)
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "1 pc in AU", factorPcAU, 206264.8, 0.5) // IAU value

	// 1 ly in pc
	factorLyPc, err := unit.LightYear.ConversionFactor(unit.Parsec)
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "1 ly in pc", factorLyPc, 0.30660, 1e-4)

	// Incompatible: AU vs second
	_, err = unit.AstronomicalUnit.ConversionFactor(unit.Second)
	testutil.AssertError(t, err)
}

func TestJansky(t *testing.T) {
	// Jansky should have SpectralFlux dimension
	testutil.AssertEqual(t, "Jansky dimension", unit.Jansky.Dimension, unit.SpectralFlux)
	testutil.AssertNear(t, "Jansky scale", unit.Jansky.ScaleFactor, 1e-26, 1e-40)

	// Jy is not compatible with length
	if unit.Jansky.Compatible(unit.Meter) {
		t.Error("Jansky should not be compatible with Meter")
	}
}

func TestUnitString(t *testing.T) {
	testutil.AssertEqual(t, "Meter symbol", unit.Meter.String(), "m")
	testutil.AssertEqual(t, "AU symbol", unit.AstronomicalUnit.String(), "AU")
	testutil.AssertEqual(t, "Jansky symbol", unit.Jansky.String(), "Jy")
}
