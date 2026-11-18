package angle_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/TuSKan/astrogo/angle"
)

// TestLatitudeAndLongitudeAreTheirAngles: LatDeg and LonDeg hold the angle
// Deg does, every Angle method reads it, and Latitude{Angle: a} is the same
// value as the constructor's.
func TestLatitudeAndLongitudeAreTheirAngles(t *testing.T) {
	t.Parallel()

	lat := angle.LatDeg(19.8207)
	lon := angle.LonDeg(-155.4681)

	if lat.Angle != angle.Deg(19.8207) || lon.Angle != angle.Deg(-155.4681) {
		t.Fatalf("LatDeg/LonDeg = %v, %v; want Deg's angles", lat.Angle, lon.Angle)
	}

	if got := lat.Degrees(); math.Abs(got-19.8207) > 1e-12 {
		t.Errorf("lat.Degrees() = %v, want 19.8207", got)
	}

	if got, want := lon.Sin(), math.Sin(angle.Deg(-155.4681).Radians()); got != want {
		t.Errorf("lon.Sin() = %v, want %v", got, want)
	}

	if (angle.Latitude{Angle: angle.Deg(19.8207)}) != lat {
		t.Error("Latitude{Angle: Deg(x)} != LatDeg(x)")
	}

	if (angle.Longitude{Angle: angle.Deg(-155.4681)}) != lon {
		t.Error("Longitude{Angle: Deg(x)} != LonDeg(x)")
	}
}

// TestLatitudeIsNotLongitude: neither type is assignable to the other, which
// is what makes a swapped pair of arguments a compile error, and a bare Angle
// is assignable to neither. An explicit conversion between the two compiles,
// as it does for any two structs with the same fields; it has to be written
// on purpose.
func TestLatitudeIsNotLongitude(t *testing.T) {
	t.Parallel()

	lat, lon, ang := reflect.TypeFor[angle.Latitude](), reflect.TypeFor[angle.Longitude](), reflect.TypeFor[angle.Angle]()

	for _, c := range []struct {
		from, to reflect.Type
	}{
		{lat, lon},
		{lon, lat},
		{ang, lat},
		{ang, lon},
	} {
		if c.from.AssignableTo(c.to) {
			t.Errorf("%v is assignable to %v", c.from, c.to)
		}
	}

	if ang.ConvertibleTo(lat) || ang.ConvertibleTo(lon) {
		t.Error("a bare Angle converts to a Latitude or Longitude; it should take Latitude{Angle: a}")
	}
}
