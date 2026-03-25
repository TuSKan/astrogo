package main

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/coords"
)

func main() {
	// 1. Retrieve M8 (Lagoon Nebula) strictly bound to core catalog limits explicitly
	m8, err := catalog.Lookup("NGC6523")
	if err != nil {
		fmt.Printf("Error looking up Lagoon mapping: %v\n", err)
		return
	}

	// 2. Retrieve M20 (Trifid Nebula) bounding array natively implicitly
	m20, err := catalog.Lookup("NGC6514")
	if err != nil {
		fmt.Printf("Error looking up Trifid boundaries natively: %v\n", err)
		return
	}

	// 3. Calculate native explicit separation using structured Topocentric mathematical mapping vectors natively across equatorial geometries natively
	separationRads := coords.AngularSeparation(m8.RA, m8.Dec, m20.RA, m20.Dec)

	// Convert exact radians purely back into human readable Arcminutes internally evaluating constraints natively
	separationArcMins := separationRads * (180.0 / math.Pi) * 60.0

	fmt.Printf("The native separation between %s and %s evaluates purely to %.2f arcminutes natively.\n", m8.ID, m20.ID, separationArcMins)
}
