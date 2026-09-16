package coord_test

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
)

// The frame is where Galactic structure is written down: a target's distance
// from us and its distance from the centre of the Galaxy are different
// questions, and only the second one organises anything.
func ExampleGalactocentricFrame_FromICRS() {
	f := coord.DefaultGalactocentricFrame()

	// Three targets a kiloparsec away, in three Galactic directions.
	for _, tc := range []struct {
		name string
		l, b float64
	}{
		{"toward the centre", 0, 0},
		{"along the rotation", 90, 0},
		{"toward the pole", 0, 90},
	} {
		icrs := coord.GalacticToICRS(coord.NewGalactic(angle.Deg(tc.l), angle.Deg(tc.b)))
		g := f.FromICRS(icrs, 1000)

		fmt.Printf("%-19s R = %7.1f pc   Z = %+7.1f pc\n", tc.name, g.Radius(), g.Z())
	}

	sun := f.SunPosition()
	fmt.Printf("%-19s R = %7.1f pc   Z = %+7.1f pc\n", "the Sun", sun.Radius(), sun.Z())

	// A kiloparsec of travel buys a full kiloparsec of R toward the centre and
	// only 61 pc of it along the rotation, which is why a rotation curve is a
	// function of R rather than of anything an observer measures directly.
	//
	// The Sun's 20.8 pc of height is visible in every row, and so is the tilt
	// it implies: toward the centre Z drops to 18.3, and toward the pole R
	// falls 2.6 pc below the Sun's. Both are 1000·sin(0.1457°). Only the
	// rotation direction is untouched, because the tilt is about that axis.

	// Output:
	// toward the centre   R =  7178.0 pc   Z =   +18.3 pc
	// along the rotation  R =  8238.9 pc   Z =   +20.8 pc
	// toward the pole     R =  8175.4 pc   Z = +1020.8 pc
	// the Sun             R =  8178.0 pc   Z =   +20.8 pc
}

// A catalogue gives a parallax, and the frame wants parsecs.
func ExampleParallaxDistance() {
	// A star at Galactic (l, b) = (120°, +15°) with a parallax of 2 mas.
	star := coord.GalacticToICRS(coord.NewGalactic(angle.Deg(120), angle.Deg(15)))

	d := coord.ParallaxDistance(angle.Arcsec(0.002))
	g := coord.DefaultGalactocentricFrame().FromICRS(star, d)

	fmt.Printf("distance from the Sun:    %.1f pc\n", d)
	fmt.Printf("distance from the centre: %.1f pc\n", g.Distance())
	fmt.Printf("height above the plane:   %+.1f pc\n", g.Z())

	// The 150.8 pc is the star's own 129.4 pc above the Sun (500·sin 15°) plus
	// the Sun's 20.8, plus 0.6 pc of the frame's tilt.

	// Output:
	// distance from the Sun:    500.0 pc
	// distance from the centre: 8430.9 pc
	// height above the plane:   +150.8 pc
}
