package sky_test

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/sky"
)

func ExampleSeparation() {
	p1 := coord.ICRS{RA: angle.Deg(0), Dec: angle.Deg(0)}
	p2 := coord.ICRS{RA: angle.Deg(0.1), Dec: angle.Deg(0)} // 6 arcminutes away

	sep := sky.Separation(p1, p2)
	fmt.Printf("%.0f arcmin\n", sep.Arcminutes())
	// Output: 6 arcmin
}

func ExampleAirmass() {
	alt := angle.Deg(30)
	am, _ := sky.Airmass(alt)

	fmt.Printf("%.2f\n", am)
	// Output: 1.99
}
