package coord_test

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
)

func ExampleICRS_ToUnitVector() {
	// Object at RA=0, Dec=0
	c := coord.ICRS{RA: angle.Deg(0), Dec: angle.Deg(0)}
	v := c.ToUnitVector()

	fmt.Printf("X: %.1f, Y: %.1f, Z: %.1f\n", v.X, v.Y, v.Z)
	// Output: X: 1.0, Y: 0.0, Z: 0.0
}

func ExampleAltAz() {
	// Altitude 45, Azimuth 180 (South)
	aa := coord.AltAz{Alt: angle.Deg(45), Az: angle.Deg(180)}
	fmt.Println(aa.Alt.DMSString(0))
	// Output: +45°00'00"
}
