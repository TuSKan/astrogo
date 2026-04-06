package unit_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/unit"
)

func TestNew(t *testing.T) {
	q := unit.New(10, unit.Meter)
	testutil.AssertNear(t, "Value", q.Value, 10, 1e-15)
	testutil.AssertEqual(t, "Unit", q.Unit, unit.Meter)
}

func TestConversion(t *testing.T) {
	q := unit.New(1000, unit.Meter)

	// Valid conversion
	km, err := q.In(unit.Kilometer)
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "1000m in km", km.Value, 1.0, 1e-15)
	testutil.AssertEqual(t, "Unit km", km.Unit, unit.Kilometer)

	// MustIn
	km2 := q.MustIn(unit.Kilometer)
	testutil.AssertNear(t, "MustIn 1000m to km", km2.Value, 1.0, 1e-15)

	// Incompatible conversion
	_, err = q.In(unit.Second)
	testutil.AssertError(t, err)
	testutil.AssertErrorContains(t, err, "incompatible unit conversion")
}

func TestArithmetic(t *testing.T) {
	d1 := unit.New(1.5, unit.Kilometer)
	d2 := unit.New(500, unit.Meter)

	// Add
	sum, err := d1.Add(d2)
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "1.5km + 500m", sum.Value, 2.0, 1e-15)
	testutil.AssertEqual(t, "Sum unit", sum.Unit, unit.Kilometer)

	// Sub
	diff, err := d1.Sub(d2)
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "1.5km - 500m", diff.Value, 1.0, 1e-15)

	// Incompatible arithmetic
	t1 := unit.New(10, unit.Second)
	_, err = d1.Add(t1)
	testutil.AssertError(t, err)
}

func TestDerivedArithmetic(t *testing.T) {
	// m * m = m^2
	side := unit.New(10, unit.Meter)
	area := side.Mul(side)
	testutil.AssertNear(t, "10m * 10m", area.Value, 100, 1e-15)
	testutil.AssertEqual(t, "Area dimension", area.Unit.Dimension, unit.Area)

	// m / s = Velocity
	dist := unit.New(100, unit.Meter)
	time := unit.New(10, unit.Second)
	vel := dist.Div(time)
	testutil.AssertNear(t, "100m / 10s", vel.Value, 10, 1e-15)
	testutil.AssertEqual(t, "Velocity dimension", vel.Unit.Dimension, unit.Velocity)
}

func TestString(t *testing.T) {
	q := unit.New(1.23, unit.Kilometer)
	testutil.AssertEqual(t, "String()", q.String(), "1.23 km")
}

func TestEquals(t *testing.T) {
	q1 := unit.New(1000, unit.Meter)
	q2 := unit.New(1, unit.Kilometer)
	q3 := unit.New(1.1, unit.Kilometer)
	q4 := unit.New(1000, unit.Second)

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
	q := unit.New(1, unit.Meter)
	_ = q.MustIn(unit.Second)
}

func TestScale(t *testing.T) {
	q := unit.New(10, unit.Meter)
	s := q.Scale(3.5)
	testutil.AssertNear(t, "Scale value", s.Value, 35, 1e-15)
	testutil.AssertEqual(t, "Scale unit", s.Unit, unit.Meter)
}

func TestAbs(t *testing.T) {
	neg := unit.New(-7.5, unit.Kilometer)
	pos := neg.Abs()
	testutil.AssertNear(t, "Abs value", pos.Value, 7.5, 1e-15)

	alreadyPos := unit.New(3.0, unit.Meter)
	testutil.AssertNear(t, "Abs already positive", alreadyPos.Abs().Value, 3.0, 1e-15)
}

func TestIsZeroNaN(t *testing.T) {
	zero := unit.New(0, unit.Meter)
	nonZero := unit.New(1, unit.Meter)
	nan := unit.New(math.NaN(), unit.Meter)

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
	a := unit.New(500, unit.Meter)
	b := unit.New(1, unit.Kilometer)
	c := unit.New(2, unit.Kilometer)

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
	_, err = a.Compare(unit.New(1, unit.Second))
	testutil.AssertError(t, err)
	_ = c // silence unused
}
