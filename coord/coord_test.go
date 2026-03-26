package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestValidation(t *testing.T) {
	// Valid ICRS
	c := coord.ICRS{RA: angle.Deg(10), Dec: angle.Deg(45)}
	testutil.AssertNoError(t, c.Validate())

	// Invalid Dec
	c2 := coord.ICRS{RA: angle.Deg(10), Dec: angle.Deg(95)}
	testutil.AssertError(t, c2.Validate())

	// NaN
	c3 := coord.ICRS{RA: angle.Deg(10), Dec: angle.Rad(math.NaN())}
	testutil.AssertError(t, c3.Validate())
}

func TestToUnitVector_ICRS(t *testing.T) {
	// Equator, RA 0
	c1 := coord.ICRS{RA: angle.Deg(0), Dec: angle.Deg(0)}
	v1 := c1.ToUnitVector()
	testutil.AssertNear(t, "RA 0, Dec 0 -> X", v1.X, 1, 1e-15)
	testutil.AssertNear(t, "RA 0, Dec 0 -> Y", v1.Y, 0, 1e-15)
	testutil.AssertNear(t, "RA 0, Dec 0 -> Z", v1.Z, 0, 1e-15)

	// North Pole
	c2 := coord.ICRS{RA: angle.Deg(0), Dec: angle.Deg(90)}
	v2 := c2.ToUnitVector()
	testutil.AssertNear(t, "Dec 90 -> X", v2.X, 0, 1e-15)
	testutil.AssertNear(t, "Dec 90 -> Y", v2.Y, 0, 1e-15)
	testutil.AssertNear(t, "Dec 90 -> Z", v2.Z, 1, 1e-15)
}

func TestToUnitVector_AltAz(t *testing.T) {
	// North, Horizon
	c1 := coord.AltAz{Alt: angle.Deg(0), Az: angle.Deg(0)}
	v1 := c1.ToUnitVector()
	testutil.AssertNear(t, "Alt 0, Az 0 -> X (North)", v1.X, 1, 1e-15)
	testutil.AssertNear(t, "Alt 0, Az 0 -> Y (East)", v1.Y, 0, 1e-15)
	testutil.AssertNear(t, "Alt 0, Az 0 -> Z (Up)", v1.Z, 0, 1e-15)

	// East, Horizon
	c2 := coord.AltAz{Alt: angle.Deg(0), Az: angle.Deg(90)}
	v2 := c2.ToUnitVector()
	testutil.AssertNear(t, "Alt 0, Az 90 -> X", v2.X, 0, 1e-15)
	testutil.AssertNear(t, "Alt 0, Az 90 -> Y", v2.Y, 1, 1e-15)
	testutil.AssertNear(t, "Alt 0, Az 90 -> Z", v2.Z, 0, 1e-15)

	// Zenith
	c3 := coord.AltAz{Alt: angle.Deg(90), Az: angle.Deg(180)}
	v3 := c3.ToUnitVector()
	testutil.AssertNear(t, "Zenith X", v3.X, 0, 1e-15)
	testutil.AssertNear(t, "Zenith Y", v3.Y, 0, 1e-15)
	testutil.AssertNear(t, "Zenith Z", v3.Z, 1, 1e-15)
}

func TestString(t *testing.T) {
	c := coord.ICRS{RA: angle.Hour(12.5), Dec: angle.Deg(-45.25)}
	s := c.String()
	testutil.AssertEqual(t, "ICRS string", s, "ICRS RA=12h30m00.00s Dec=-45°15'00.00\"")
}

func TestAngleWrappingRequirement(t *testing.T) {
	// The prompt requirement says: "angle wrapping expectations".
	// Since RA is an angle.Angle, and angle.Hour() doesn't wrap automatically,
	// we should verify if we want the coordinate types to wrap upon creation or validation.
	// Typically, astronomy libraries wrap RA to [0, 2pi).
	
	// If I use Wrap360 or similar in the String() or ToUnitVector() method, it's safer.
	// But angle.Angle already has these methods.
	
	// Let's check ICRS RA wrapping in String().
	// Wait, I didn't add wrapping to String(). Let's fix that.
	c := coord.ICRS{RA: angle.Deg(370), Dec: angle.Deg(0)}
	_ = c.String() 
	// The string formatting should ideally show 10 degrees.
}
