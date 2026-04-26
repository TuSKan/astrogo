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
	loc, err := coord.NewEarthLocation(-23.5505, -46.6333, 760.0)
	if err != nil {
		log.Fatalf("failed to create location: %v", err)
	}

	tz, _ := time.LoadLocation("UTC")
	t := time.Date(2026, 4, 25, 20, 0, 0, 0, tz)
	ctx := coord.NewContext(t, loc, atmosphere.StandardAtmosphere)

	prov, err := eph.NewProvider(eph.Planets, "de442")
	if err != nil {
		log.Fatalf("failed to create provider: %v", err)
	}
	mars := plan.NewMars(prov)

	details, err := mars.GetDetails(ctx, "Description", "The Red Planet", "Source", "JPL DE442")
	if err != nil {
		log.Fatalf("failed to get details: %v", err)
	}

	fmt.Println(details)
}
