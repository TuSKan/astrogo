package sky_test

import (
	"fmt"
	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/sky"
)

func ExampleSeparation() {
	p1 := sky.NewTarget("Star A", 0, 0)
	p2 := sky.NewTarget("Star B", 0.1, 0) // 6 arcminutes away
	
	sep := sky.Separation(p1.Coord, p2.Coord)
	fmt.Printf("%.0f arcmin\n", sep.Arcminutes())
	// Output: 6 arcmin
}

func ExampleAirmass() {
	alt := angle.Deg(30)
	am, _ := sky.Airmass(alt)
	
	fmt.Printf("%.2f\n", am)
	// Output: 1.99
}
