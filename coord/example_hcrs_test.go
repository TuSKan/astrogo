package coord_test

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// Whether the origin matters is a question about distance, not about the
// frame: the same shift is a quarter of a degree for a planet and eight
// thousandths of an arcsecond for a star a parsec away.
//
// How much of the shift is visible also depends on where the target is
// relative to the Sun's own offset, so these are below the worst case the
// distance allows -- 1099 arcsec and 0.0091 arcsec respectively.
func ExampleBarycentricToHeliocentric() {
	ep := time.Date(2023, 1, 1, 0, 0, 0, 0, time.LocationUTC)

	sun, err := coord.SunBarycentric(ep)
	if err != nil {
		fmt.Println(err)

		return
	}

	fmt.Printf("the Sun is %.6f AU from the barycentre\n", sun.Norm())

	// A body about where Mars is, and a star about a parsec away, both given
	// barycentrically.
	for _, tc := range []struct {
		name string
		v    vector.Vec3
	}{
		{"a body at 1.7 AU", vector.V3(0.5053, -1.7138, -0.9949)},
		{"a star at 1 parsec", vector.V3(104253, -145127, -84257)},
	} {
		helio, err := coord.BarycentricToHeliocentric(tc.v, ep)
		if err != nil {
			fmt.Println(err)

			return
		}

		// How far the direction moved, as an angle: the cross product over
		// the dot product is the same atan2 [coord.Separation] uses, and works
		// here for vectors that are not unit length.
		moved := angle.Atan2(tc.v.Cross(helio).Norm(), tc.v.Dot(helio))

		fmt.Printf("%-20s direction moves %9.4f arcsec\n", tc.name, moved.Arcseconds())
	}

	// Output:
	// the Sun is 0.009057 AU from the barycentre
	// a body at 1.7 AU     direction moves  881.1538 arcsec
	// a star at 1 parsec   direction moves    0.0080 arcsec
}
