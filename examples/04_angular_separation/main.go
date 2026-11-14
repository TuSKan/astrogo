// Package main demonstrates angular separation calculation.
package main

import (
	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/examples/internal/show"
)

func main() {
	// 1. Coordinates for the first star (Alcor, in Ursa Major)
	// RA: 13h 25m 13.5s, Dec: +54° 59′ 16″
	alcor := coord.NewICRS(angle.Hour(13.4204), angle.Deg(54.9878))

	// 2. Coordinates for the second star (Mizar, its neighbor)
	// RA: 13h 23m 55.5s, Dec: +54° 55′ 31″
	mizar := coord.NewICRS(angle.Hour(13.3987), angle.Deg(54.9253))

	// 3. Compute the angular separation: the great-circle angle between them
	sep := coord.Separation(alcor, mizar)

	show.Title("Angular separation", "Mizar and Alcor, in the handle of the Big Dipper")
	show.Fields(
		show.KV("Alcor", "RA %s   Dec %s", alcor.RA().HMSString(1), alcor.Dec().DMSString(0)),
		show.KV("Mizar", "RA %s   Dec %s", mizar.RA().HMSString(1), mizar.Dec().DMSString(0)),
		show.KV("Separation", "%.2f arcmin (%.4f°)", sep.Arcminutes(), sep.Degrees()),
	)
}
