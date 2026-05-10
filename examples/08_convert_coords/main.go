package main

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	// 1. Start with an ICRS coordinate (e.g., The Galactic Center - Sgr A*)
	icrs := coord.NewICRS(angle.Hour(17.7611), angle.Deg(-28.9856))

	// 2. Convert ICRS -> Galactic
	galactic := coord.ICRSToGalactic(icrs)

	// 3. Convert ICRS -> AltAz (requires Site details and Time)
	loc, _ := coord.NewEarthLocation(-23.5505, -46.6333, 760) // São Paulo
	now := time.NowUTC()

	ctx := coord.NewContext(now, loc, atmosphere.StandardAtmosphere)

	altaz, err := ctx.ICRSToAltAz(icrs)
	if err != nil {
		fmt.Printf("Error converting to AltAz: %v\n", err)
	}

	// 4. Print outputs!
	fmt.Println("Coordinate Framework Conversion")
	fmt.Println("===============================")
	fmt.Printf("Object:   %s\n", icrs)
	fmt.Printf("Galactic: l=%.4f° b=%.4f° \n", galactic.L().Degrees(), galactic.B().Degrees())
	fmt.Printf("AltAz:    Alt=%.4f° Az=%.4f° (from São Paulo at %v)\n", altaz.Alt().Degrees(), altaz.Az().Degrees(), now.ToGo().Format("15:04 UTC"))
}
