package main

import (
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	// 1. Define the observer's location (São Paulo, Brazil)
	loc, err := coord.NewEarthLocation(-23.5505, -46.6333, 760)
	if err != nil {
		log.Fatal(err)
	}

	// 2. Set the time to 'tonight at 7 PM' in local timezone (UTC-3)
	tz, _ := time.LoadLocation("America/Sao_Paulo")
	tm := time.Date(2026, 4, 6, 19, 0, 0, 0, tz)

	// 3. Create a moving target for Mars using the built-in default ephemeris
	mars := plan.NewMars(eph.Default())

	// 4. Get the geocentric ICRS coordinates of Mars at this exact time
	icrs, err := mars.Position(tm)
	if err != nil {
		log.Fatalf("Error computing position: %v", err)
	}

	// 5. Convert ICRS sky coordinates to local Altitude and Azimuth
	ctx := coord.NewContext(tm, loc, atmosphere.StandardAtmosphere)
	skyPos, err := ctx.ICRSToAltAz(icrs)
	if err != nil {
		log.Fatalf("Error converting to Alt/Az: %v", err)
	}

	fmt.Printf("Time: %s (%s)\n", tm, tm.Format("2006-01-02 15:04:05 -0700"))
	fmt.Printf("Observer: São Paulo, Brazil\n")
	fmt.Printf("Mars Altitude: %.2f°\n", skyPos.Alt().Degrees())
	fmt.Printf("Mars Azimuth:  %.2f°\n", skyPos.Az().Degrees())
}
