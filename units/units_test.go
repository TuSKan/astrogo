package units_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/units"
)

func TestDimensionAlgebra(t *testing.T) {
	// L * L = L^2
	areaDim := units.Length.Mul(units.Length)
	testutil.AssertEqual(t, "Length*Length == Area", areaDim, units.Area)

	// L / T = Velocity
	velDim := units.Length.Div(units.Time)
	testutil.AssertEqual(t, "Length/Time == Velocity", velDim, units.Velocity)

	// L^3 = Volume
	volDim := units.Length.PowInt(3)
	testutil.AssertEqual(t, "Length^3 == Volume", volDim, units.Volume)

	// Inquality
	if units.Length.Equals(units.Mass) {
		t.Error("Length should not equal Mass")
	}
}

func TestUnitCompatibility(t *testing.T) {
	if !units.Meter.Compatible(units.Kilometer) {
		t.Error("Meter should be compatible with Kilometer")
	}
	if units.Meter.Compatible(units.Second) {
		t.Error("Meter should not be compatible with Second")
	}
}

func TestConversionFactors(t *testing.T) {
	cases := []struct {
		from   units.Unit
		to     units.Unit
		factor float64
	}{
		{units.Kilometer, units.Meter, 1000},
		{units.Meter, units.Kilometer, 0.001},
		{units.Hour, units.Second, 3600},
		{units.Second, units.Minute, 1.0 / 60.0},
		{units.Degree, units.Radian, math.Pi / 180.0},
		{units.Arcminute, units.Degree, 1.0 / 60.0},
	}

	for i, c := range cases {
		factor, err := c.from.ConversionFactor(c.to)
		testutil.AssertNoError(t, err)
		testutil.AssertNear(t, testutil.CaseLabel(i, c.from.Symbol+" to "+c.to.Symbol), factor, c.factor, 1e-15)
	}
}

func TestIncompatibleConversion(t *testing.T) {
	_, err := units.Meter.ConversionFactor(units.Second)
	testutil.AssertError(t, err)
	testutil.AssertErrorContains(t, err, "incompatible unit conversion")
}

func TestDerivedUnits(t *testing.T) {
	// m/s^2 (Acceleration)
	accel := units.Meter.Div(units.Second.PowInt(2))
	testutil.AssertEqual(t, "m/s^2 dimension", accel.Dimension, units.Accel)
	testutil.AssertNear(t, "m/s^2 scale", accel.ScaleFactor, 1.0, 1e-15)

	// km/h (Velocity)
	kmph := units.Kilometer.Div(units.Hour)
	testutil.AssertEqual(t, "km/h dimension", kmph.Dimension, units.Velocity)
	// 1000 m / 3600 s = 1/3.6 m/s
	testutil.AssertNear(t, "km/h scale", kmph.ScaleFactor, 1000.0/3600.0, 1e-15)
}

func TestUnitAlgebra(t *testing.T) {
	m2 := units.Meter.Mul(units.Meter)
	testutil.AssertEqual(t, "m*m dimension", m2.Dimension, units.Area)
	testutil.AssertNear(t, "m*m scale", m2.ScaleFactor, 1.0, 1e-15)

	newton := units.Kilogram.Mul(units.Meter).Div(units.Second.PowInt(2))
	testutil.AssertEqual(t, "Newton dimension", newton.Dimension, units.Force)
}

func TestAstronomicalUnits(t *testing.T) {
	// 1 AU in km
	factor, err := units.AstronomicalUnit.ConversionFactor(units.Kilometer)
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "1 AU in km", factor, 1.495978707e8, 1e3) // ~1.496e8 km

	// 1 pc in AU
	factorPcAU, err := units.Parsec.ConversionFactor(units.AstronomicalUnit)
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "1 pc in AU", factorPcAU, 206264.8, 0.5) // IAU value

	// 1 ly in pc
	factorLyPc, err := units.LightYear.ConversionFactor(units.Parsec)
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "1 ly in pc", factorLyPc, 0.30660, 1e-4)

	// Incompatible: AU vs second
	_, err = units.AstronomicalUnit.ConversionFactor(units.Second)
	testutil.AssertError(t, err)
}

func TestJansky(t *testing.T) {
	// Jansky should have SpectralFlux dimension
	testutil.AssertEqual(t, "Jansky dimension", units.Jansky.Dimension, units.SpectralFlux)
	testutil.AssertNear(t, "Jansky scale", units.Jansky.ScaleFactor, 1e-26, 1e-40)

	// Jy is not compatible with length
	if units.Jansky.Compatible(units.Meter) {
		t.Error("Jansky should not be compatible with Meter")
	}
}

func TestUnitString(t *testing.T) {
	testutil.AssertEqual(t, "Meter symbol", units.Meter.String(), "m")
	testutil.AssertEqual(t, "AU symbol", units.AstronomicalUnit.String(), "AU")
	testutil.AssertEqual(t, "Jansky symbol", units.Jansky.String(), "Jy")
}
