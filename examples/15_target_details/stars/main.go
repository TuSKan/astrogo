// Package main demonstrates star target details.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	// OpenNGC is listed alongside SIMBAD below. Enabling downloads here is
	// enough — catalog.NewResolver's first use of catalog.OpenNGC fetches
	// it automatically (content-checked, so a re-run only costs a HEAD
	// probe once cached).
	remote.EnableDownloads(remote.OpenNGC, 5<<20) // ~2 MB combined source CSVs

	loc, err := coord.NewEarthLocation(-23.5505, -46.6333, 760.0)
	if err != nil {
		log.Fatalf("failed to create location: %v", err)
	}

	tz, _ := time.LoadLocation("America/Sao_Paulo")
	t := time.Date(2026, 4, 25, 23, 0, 0, 0, tz)
	ctx := coord.NewContext(t, loc, atmosphere.StandardAtmosphere)

	resolver := catalog.NewResolver(catalog.OpenNGC, catalog.SIMBAD)

	catTarget, err := resolver.Resolve(context.Background(), "Sirius")
	if err != nil {
		log.Fatalf("failed to resolve Sirius: %v", err)
	}

	sirius := plan.FromCatalog(catTarget, nil)

	details, err := sirius.GetDetails(ctx,
		"Bayer letter", "α CMa",
		"Flamsteed number", "9 CMa",
		"FK5 number", "FK5 257",
		"BSC5 number", "HR 2491",
		"Hipparcos number", "HIP 32349",
		"Tycho-2 number", "TYC 5949-02777-1",
		"Designation for variable", "NSV 17173",
		"TDSC number", "TDSC 16356",
		"WDS number", "WDS 06451-1643",
		"Spectral type", "A1",
		"Luminosity class", "V - dwarfs/main sequence",
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(details)
}
