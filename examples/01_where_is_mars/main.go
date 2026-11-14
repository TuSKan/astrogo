// Package main demonstrates locating Mars in the sky.
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
	// This example runs offline, so it uses no Earth-orientation data
	// (UT1 = UTC, no polar motion). That can move an alt/az position by up
	// to 13.5 arcsec, well below the 0.01° printed here. For full accuracy,
	// blank-import remote/eop and consent to the IERS bulletin, as
	// 09_geometry_events does.
	time.RegisterModel(time.ZeroModel{})

	// 1. Define the observer's location (Quinta Calixto, Brazil)
	loc, err := coord.NewEarthLocation(-22.528478, -46.473002, 835.05)
	if err != nil {
		log.Fatal(err)
	}

	// 2. Set the time: 10 PM local time (UTC-3) on 2027-02-19, the night of
	// Mars's opposition, when it is up all night. A fixed instant keeps the
	// output the same on every run; time.NowUTC() asks about right now.
	tz, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		log.Fatalf("tz: %v", err)
	}

	tm := time.Date(2027, 2, 19, 22, 0, 0, 0, tz)

	// 3. Create a moving target for Mars using the built-in default ephemeris
	mars := plan.NewMars(eph.Default())

	// 4. Get the geocentric ICRS coordinates of Mars at this exact time
	icrs, err := mars.Position(tm)
	if err != nil {
		log.Fatalf("Error computing position: %v", err)
	}

	// 5. Convert ICRS sky coordinates to local Altitude and Azimuth
	ctx := coord.NewContext(tm, loc, atmosphere.StandardRefraction())

	skyPos, err := ctx.ICRSToAltAz(icrs)
	if err != nil {
		log.Fatalf("Error converting to Alt/Az: %v", err)
	}

	fmt.Printf("Time: %s (%s)\n", tm, tm.Format("2006-01-02 15:04:05 -0700"))
	fmt.Printf("Observer: Quinta Calixto, Brazil\n")
	fmt.Printf("Mars Altitude: %.2f°\n", skyPos.Alt().Degrees())
	fmt.Printf("Mars Azimuth:  %.2f°\n", skyPos.Az().Degrees())
}
