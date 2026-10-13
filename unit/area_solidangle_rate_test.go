package unit_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/unit"
)

// near reports whether got is within a relative tol of want.
func near(got, want, tol float64) bool {
	return math.Abs(got-want) <= tol*math.Abs(want)
}

// TestAreaConversions holds Area to its two units, cm² being 1e-4 m², and
// to the round trip through each, through Quantity and back with AreaFrom.
func TestAreaConversions(t *testing.T) {
	t.Parallel()

	if got := unit.SquareCentimeters(1).SquareMeters(); got != 1e-4 {
		t.Errorf("1 cm² = %g m², want 1e-4", got)
	}

	// An ozone cross section as a laboratory table prints it, in cm².
	sigma := unit.SquareCentimeters(4.5e-21)
	if got := sigma.SquareCentimeters(); !near(got, 4.5e-21, 1e-15) {
		t.Errorf("SquareCentimeters round trip = %g, want 4.5e-21", got)
	}

	area := unit.SquareMeters(0.785)

	back, err := unit.AreaFrom(area.Quantity())
	if err != nil || back != area {
		t.Errorf("AreaFrom(Quantity()) = %v, %v; want %v", back, err, area)
	}

	if _, err := unit.AreaFrom(unit.Meters(1).Quantity()); err == nil {
		t.Error("AreaFrom accepted a length")
	} else if !errors.As(err, new(unit.IncompatibleUnitError)) {
		t.Errorf("AreaFrom(length) error = %v, want an IncompatibleUnitError", err)
	}

	for _, c := range []struct {
		a    unit.Area
		want string
	}{
		{unit.SquareMeters(0.785), "0.785 m²"},
		{unit.SquareCentimeters(4.5e-21), "4.5e-21 cm²"},
	} {
		if got := c.a.String(); got != c.want {
			t.Errorf("String() = %q, want %q", got, c.want)
		}
	}
}

// TestSolidAngleConversions holds SolidAngle to its units: a square
// arcsecond is (π/648000)² sr, 2.350443e-11, the factor every surface
// brightness per arcsec² is converted by.
func TestSolidAngleConversions(t *testing.T) {
	t.Parallel()

	if got := unit.SquareArcseconds(1).Steradians(); !near(got, 2.3504430539097885e-11, 1e-15) {
		t.Errorf("1 arcsec² = %.16g sr, want 2.3504430539097885e-11", got)
	}

	// A whole sphere is 4π sr, 5.346e11 square arcseconds.
	if got := unit.Steradians(4 * math.Pi).SquareArcseconds(); !near(got, 4*math.Pi/2.3504430539097885e-11, 1e-12) {
		t.Errorf("4π sr = %g arcsec², want %g", got, 4*math.Pi/2.3504430539097885e-11)
	}

	pixel := unit.SquareArcseconds(0.25)
	if q := pixel.Quantity(); q.Unit != unit.Steradian || !near(q.Value, pixel.Steradians(), 0) {
		t.Errorf("Quantity() = %v, want %g sr", q, pixel.Steradians())
	}

	for _, c := range []struct {
		s    unit.SolidAngle
		want string
	}{
		{unit.Steradians(2 * math.Pi), "6.28319 sr"},
		{unit.SquareArcseconds(0.25), "0.25 arcsec²"},
	} {
		if got := c.s.String(); got != c.want {
			t.Errorf("String() = %q, want %q", got, c.want)
		}
	}
}

// TestAngularRateConversions holds AngularRate to its units: a degree per
// second is π/180 rad/s, and a revolution a day is 2π/86400 rad/s, so the
// ISS's 15.5 revolutions a day is 1.127e-3 rad/s.
func TestAngularRateConversions(t *testing.T) {
	t.Parallel()

	if got := unit.DegreesPerSec(1).RadiansPerSec(); !near(got, math.Pi/180, 1e-15) {
		t.Errorf("1°/s = %g rad/s, want π/180", got)
	}

	iss := unit.RevolutionsPerDay(15.5)
	if got := iss.RadiansPerSec(); !near(got, 15.5*2*math.Pi/86400, 1e-15) {
		t.Errorf("15.5 rev/d = %g rad/s, want %g", got, 15.5*2*math.Pi/86400)
	}

	if got := iss.RevolutionsPerDay(); !near(got, 15.5, 1e-15) {
		t.Errorf("RevolutionsPerDay round trip = %g, want 15.5", got)
	}

	slew := unit.DegreesPerSec(2)
	if got := slew.DegreesPerSec(); !near(got, 2, 1e-15) {
		t.Errorf("DegreesPerSec round trip = %g, want 2", got)
	}

	if q := slew.Quantity(); q.Unit != unit.RadianPerSecond || !near(q.Value, slew.RadiansPerSec(), 0) {
		t.Errorf("Quantity() = %v, want %g rad/s", q, slew.RadiansPerSec())
	}

	if got, want := slew.String(), "2°/s"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
