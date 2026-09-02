// Package main demonstrates deep-sky object target details.
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
	remote.EnableDownloads(5<<20, remote.OpenNGC) // ~2 MB combined source CSVs

	loc, err := coord.NewEarthLocation(-23.5505, -46.6333, 760.0)
	if err != nil {
		log.Fatalf("failed to create location: %v", err)
	}

	tz, err := time.LoadLocation("UTC")
	if err != nil {
		log.Fatalf("tz: %v", err)
	}

	t := time.Date(2026, 4, 25, 20, 0, 0, 0, tz)
	ctx := coord.NewContext(t, loc, atmosphere.StandardRefraction)

	resolver := catalog.NewResolver(catalog.OpenNGC, catalog.SIMBAD)

	catTarget, err := resolver.Resolve(context.Background(), "M31")
	if err != nil {
		log.Fatalf("failed to resolve M31: %v", err)
	}

	m31, err := plan.FromCatalog(catTarget, nil)
	if err != nil {
		log.Fatalf("build target: %v", err)
	}

	details, err := m31.GetDetails(ctx, "Description", "Andromeda Galaxy", "Source", "OpenNGC")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(details)
}
