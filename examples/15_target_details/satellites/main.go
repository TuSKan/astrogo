// Package main demonstrates satellite target details.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
)

func main() {
	loc, err := coord.NewEarthLocation(-23.5505, -46.6333, 760.0)
	if err != nil {
		log.Fatalf("failed to create location: %v", err)
	}

	// We'll use the satellite epoch for context later or current time
	resolver := catalog.NewResolver(catalog.NORAD)

	catTarget, err := resolver.Resolve("ISS (Zarya)")
	if err != nil {
		log.Fatalf("failed to resolve ISS: %v", err)
	}

	// For satellites, we create an ephemeris provider using TLE strings
	prov, err := eph.NewProvider(context.Background(), eph.Satellites, catTarget.Name, eph.WithTLE(catTarget.TLELine1, catTarget.TLELine2))
	if err != nil {
		log.Fatalf("failed to create satellite provider: %v", err)
	}
	defer func() {
		err := prov.Close()
		if err != nil {
			log.Printf("failed to close provider: %v", err)
		}
	}()

	iss := plan.FromCatalog(catTarget, prov)

	// Use satellite epoch for calculation (you can use time.Date/Now() as well)
	t := catTarget.Epoch
	ctx := coord.NewContext(t, loc, atmosphere.StandardAtmosphere)

	details, err := iss.GetDetails(ctx, "Description", "International Space Station", "Source", "NORAD TLE")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(details)
}
