package main

import (
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	// 1. Setup Observatory (São Paulo, Brazil with precise coordinates from user's app)
	// Lat: 23° 36' 03'' S = -23.600833°
	// Lon: 46° 39' 09'' W = -46.6525°
	// Elev: 786m
	loc, _ := coord.NewGeodetic(angle.Deg(-46.6525), angle.Deg(-23.600833), 786)
	site, _ := plan.NewSite("São Paulo", loc, angle.Zero(), nil)

	// 2. Define Constraints: The target must be above 30 degrees altitude
	constraints := []plan.Constraint{
		plan.Altitude{Threshold: angle.Deg(30)},
	}

	// Create a planner with the site and constraints
	planner, _ := plan.NewPlanner(site, constraints)

	// 3. Set Target using Default Local Catalog (OpenNGC)
	target, err := plan.NewDefaultFixed("Orion Nebula")
	if err != nil {
		log.Fatalf("Failed to resolve target: %v", err)
	}

	// 4. Set Time to 'tonight at 7 PM' (UTC-3)
	tz, _ := time.LoadLocation("America/Sao_Paulo")
	tm := time.Date(2026, 4, 6, 19, 0, 0, 0, tz)

	// 5. Check Visibility!
	visible, reasons := planner.Observable(target, tm)
	fmt.Printf("Checking visibility of %s at %v from %s...\n\n", target.Name(), tm.ToGo().In(tz).Format("15:04 -0700"), site.Name())

	if visible {
		fmt.Printf("Result: Yes! %s is visible right now and satisfies all constraints.\n", target.Name())
	} else {
		fmt.Printf("Result: No. %s is not currently observable.\n", target.Name())
		fmt.Printf("Reasons behind this: %v\n", reasons)
	}
}
