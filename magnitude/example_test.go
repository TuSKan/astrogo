package magnitude_test

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/magnitude"
)

// The task this package exists for: how bright is a thing, as seen from here,
// now — which is never quite the number in the catalogue.
//
// For a star the catalogue magnitude is above the atmosphere, so the only
// correction is extinction, and it is proportional to airmass.
func Example() {
	// Sirius, V = −1.46, seen at 20° altitude.
	airmass, err := atmosphere.Airmass(angle.Deg(20))
	if err != nil {
		panic(err)
	}

	fmt.Printf("airmass  %.3f\n", airmass)
	fmt.Printf("catalogue -1.46, observed %.2f\n", magnitude.StarApparent(-1.46, airmass))

	// Output:
	// airmass  2.900
	// catalogue -1.46, observed -0.88
}

// A Solar System body has no fixed magnitude at all: its brightness depends on
// how far it is from the Sun, how far from us, and what phase angle it is seen
// at. The H,G system is the IAU's standard answer for asteroids.
func ExampleAsteroidHG() {
	// 1 Ceres: H = 3.34, G = 0.12. At opposition it is 2.77 AU from the Sun
	// and 1.77 from us, with a phase angle near zero.
	opposition := magnitude.AsteroidHG(3.34, 0.12, 2.77, 1.77, angle.Deg(0))

	// The same body at quadrature, further away and lit from the side.
	quadrature := magnitude.AsteroidHG(3.34, 0.12, 2.77, 2.60, angle.Deg(20))

	// Checked by hand: 3.34 + 5*log10(2.77*1.77) = 6.79 at zero phase, and the
	// 20 deg case adds ~1.0 mag of phase darkening on top of the extra distance.
	fmt.Printf("opposition  %.2f\n", opposition)
	fmt.Printf("quadrature  %.2f\n", quadrature)

	// Output:
	// opposition  6.79
	// quadrature  8.67
}
