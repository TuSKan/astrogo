// Package main demonstrates target visibility analysis.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	// 1. Setup Observatory (Quinta Calixto, Brazil)
	site, err := plan.NewSiteEarthLocation("Quinta Calixto", -22.528478, -46.473002, 835.05)
	if err != nil {
		log.Fatalf("site: %v", err)
	}

	// 2. Define Constraints: The target must be above 30 degrees altitude
	constraints := []plan.Constraint{
		plan.Altitude{Threshold: angle.Deg(30)},
	}

	// Create a planner with the site and constraints
	planner, err := plan.NewPlanner(site, constraints)
	if err != nil {
		log.Fatalf("planner: %v", err)
	}

	// 3. Set Target using SIMBAD
	targetData, err := catalog.NewResolver(catalog.SIMBAD).Resolve(context.Background(), "Orion Nebula")
	if err != nil {
		log.Fatalf("Failed to resolve target: %v", err)
	}

	target := plan.FromCatalog(targetData, nil)

	// 4. Set Time to 'tonight at 7 PM' (UTC-3)
	tz, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		log.Fatalf("tz: %v", err)
	}

	tm := time.Date(2026, 4, 6, 19, 0, 0, 0, tz)

	// 5. Check Visibility!
	visible, reasons := planner.Observable(target, tm)
	fmt.Printf("Checking visibility of %s at %v from %s...\n\n", target.Name(), tm.Format("15:04 -0700"), site.Name())

	if visible {
		fmt.Printf("Result: Yes! %s is visible right now and satisfies all constraints.\n", target.Name())
	} else {
		fmt.Printf("Result: No. %s is not currently observable.\n", target.Name())
		fmt.Printf("Reasons behind this: %v\n", reasons)
	}
}
