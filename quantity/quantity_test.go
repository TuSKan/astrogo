package quantity_test

import (
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/quantity"
	"github.com/TuSKan/astrogo/units"
)

func TestNew(t *testing.T) {
	q := quantity.New(10, units.Meter)
	testutil.AssertNear(t, "Value", q.Value, 10, 1e-15)
	testutil.AssertEqual(t, "Unit", q.Unit, units.Meter)
}

func TestConversion(t *testing.T) {
	q := quantity.New(1000, units.Meter)

	// Valid conversion
	km, err := q.In(units.Kilometer)
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "1000m in km", km.Value, 1.0, 1e-15)
	testutil.AssertEqual(t, "Unit km", km.Unit, units.Kilometer)

	// MustIn
	km2 := q.MustIn(units.Kilometer)
	testutil.AssertNear(t, "MustIn 1000m to km", km2.Value, 1.0, 1e-15)

	// Incompatible conversion
	_, err = q.In(units.Second)
	testutil.AssertError(t, err)
	testutil.AssertErrorContains(t, err, "incompatible unit conversion")
}

func TestArithmetic(t *testing.T) {
	d1 := quantity.New(1.5, units.Kilometer)
	d2 := quantity.New(500, units.Meter)

	// Add
	sum, err := d1.Add(d2)
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "1.5km + 500m", sum.Value, 2.0, 1e-15)
	testutil.AssertEqual(t, "Sum unit", sum.Unit, units.Kilometer)

	// Sub
	diff, err := d1.Sub(d2)
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "1.5km - 500m", diff.Value, 1.0, 1e-15)

	// Incompatible arithmetic
	t1 := quantity.New(10, units.Second)
	_, err = d1.Add(t1)
	testutil.AssertError(t, err)
}

func TestDerivedArithmetic(t *testing.T) {
	// m * m = m^2
	side := quantity.New(10, units.Meter)
	area := side.Mul(side)
	testutil.AssertNear(t, "10m * 10m", area.Value, 100, 1e-15)
	testutil.AssertEqual(t, "Area dimension", area.Unit.Dimension, units.Area)

	// m / s = Velocity
	dist := quantity.New(100, units.Meter)
	time := quantity.New(10, units.Second)
	vel := dist.Div(time)
	testutil.AssertNear(t, "100m / 10s", vel.Value, 10, 1e-15)
	testutil.AssertEqual(t, "Velocity dimension", vel.Unit.Dimension, units.Velocity)
}

func TestString(t *testing.T) {
	q := quantity.New(1.23, units.Kilometer)
	testutil.AssertEqual(t, "String()", q.String(), "1.23 km")
}

func TestEquals(t *testing.T) {
	q1 := quantity.New(1000, units.Meter)
	q2 := quantity.New(1, units.Kilometer)
	q3 := quantity.New(1.1, units.Kilometer)
	q4 := quantity.New(1000, units.Second)

	if !q1.Equals(q2) {
		t.Error("1000m should equal 1km")
	}
	if q1.Equals(q3) {
		t.Error("1000m should not equal 1.1km")
	}
	if q1.Equals(q4) {
		t.Error("1000m should not equal 1000s")
	}
}

func TestMustInPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("MustIn did not panic on incompatible units")
		}
	}()
	q := quantity.New(1, units.Meter)
	_ = q.MustIn(units.Second)
}
