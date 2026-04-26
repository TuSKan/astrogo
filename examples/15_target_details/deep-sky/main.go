package main

import (
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/coord"
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

	resolver := catalog.NewResolver(catalog.OpenNGC, catalog.SIMBAD)
	catTarget, err := resolver.Resolve("M31")
	if err != nil {
		log.Fatalf("failed to resolve M31: %v", err)
	}
	
	m31 := plan.NewTarget(catTarget, nil)
	details, err := m31.GetDetails(ctx, "Description", "Andromeda Galaxy", "Source", "OpenNGC")
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Println(details)
}
