package main

import (
	"fmt"
	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
)

func main() {
	// 1. Coordinates for the first star (e.g., Alcor in Ursa Major)
	// RA: 13h 25m 13.5s, Dec: +54° 59′ 16″
	alcor := coord.NewICRS(angle.Hour(13.4204), angle.Deg(54.9878))

	// 2. Coordinates for the second star (e.g., Mizar in Ursa Major)
	// RA: 13h 23m 55.5s, Dec: +54° 55′ 31″
	mizar := coord.NewICRS(angle.Hour(13.3987), angle.Deg(54.9253))

	// 3. Compute Angular Separation
	sep := coord.Separation(alcor, mizar)

	fmt.Printf("Alcor coordinates: %s\n", alcor)
	fmt.Printf("Mizar coordinates: %s\n\n", mizar)

	fmt.Printf("These two stars are %.2f arcminutes (%.4f degrees) apart in the sky.\n", sep.Arcminutes(), sep.Degrees())
}
