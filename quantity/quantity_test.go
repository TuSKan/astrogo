package quantity_test

import (
	"math"
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

func TestScale(t *testing.T) {
	q := quantity.New(10, units.Meter)
	s := q.Scale(3.5)
	testutil.AssertNear(t, "Scale value", s.Value, 35, 1e-15)
	testutil.AssertEqual(t, "Scale unit", s.Unit, units.Meter)
}

func TestAbs(t *testing.T) {
	neg := quantity.New(-7.5, units.Kilometer)
	pos := neg.Abs()
	testutil.AssertNear(t, "Abs value", pos.Value, 7.5, 1e-15)

	alreadyPos := quantity.New(3.0, units.Meter)
	testutil.AssertNear(t, "Abs already positive", alreadyPos.Abs().Value, 3.0, 1e-15)
}

func TestIsZeroNaN(t *testing.T) {
	zero := quantity.New(0, units.Meter)
	nonZero := quantity.New(1, units.Meter)
	nan := quantity.New(math.NaN(), units.Meter)

	if !zero.IsZero() {
		t.Error("IsZero: expected true for 0m")
	}
	if nonZero.IsZero() {
		t.Error("IsZero: expected false for 1m")
	}
	if !nan.IsNaN() {
		t.Error("IsNaN: expected true for NaN")
	}
	if zero.IsNaN() {
		t.Error("IsNaN: expected false for 0")
	}
}

func TestCompare(t *testing.T) {
	a := quantity.New(500, units.Meter)
	b := quantity.New(1, units.Kilometer)
	c := quantity.New(2, units.Kilometer)

	cmp, err := a.Compare(b) // 500m vs 1000m
	testutil.AssertNoError(t, err)
	if cmp != -1 {
		t.Errorf("Compare: expected -1, got %d", cmp)
	}

	cmp2, err := b.Compare(a) // 1000m vs 500m
	testutil.AssertNoError(t, err)
	if cmp2 != 1 {
		t.Errorf("Compare: expected +1, got %d", cmp2)
	}

	cmp3, err := b.Compare(b) // equal
	testutil.AssertNoError(t, err)
	if cmp3 != 0 {
		t.Errorf("Compare: expected 0, got %d", cmp3)
	}

	// Incompatible units
	_, err = a.Compare(quantity.New(1, units.Second))
	testutil.AssertError(t, err)
	_ = c // silence unused
}
