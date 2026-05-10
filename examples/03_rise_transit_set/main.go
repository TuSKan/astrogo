// Package main demonstrates rise, transit, and set computation.
package main

import (
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	// 1. Setup Observatory (São Paulo, Brazil)
	loc, _ := coord.NewEarthLocation(-23.5505, -46.6333, 760)
	site, _ := plan.NewSite("São Paulo", loc, 0, nil)

	// 2. Set a Deep Space Target
	sirius, err := catalog.NewResolver(catalog.SIMBAD).Resolve("Sirius")
	if err != nil {
		log.Fatalf("Failed to resolve target: %s", "Sirius")
	}

	target := plan.FromCatalog(sirius, nil)

	// 3. Define the Time interval (next 24 hours starting from 6 PM tonight)
	tz, _ := time.LoadLocation("America/Sao_Paulo")
	start := time.Date(2026, 4, 6, 18, 0, 0, 0, tz)
	end := start.Add(24 * time.Hour)

	// 4. Find Rise/Set/Transit events.
	// The threshold is computed automatically from the site's elevation,
	// accounting for standard atmospheric refraction and horizon dip.
	events, err := plan.VisibilityEvents(start, end, target, site)
	if err != nil {
		fmt.Printf("Error finding events: %v\n", err)
		return
	}

	fmt.Printf("Events for %s from %s at %s over 24 hours:\n\n", target.Name(), site.Name(), start)

	for _, e := range events {
		fmt.Printf("- %-10s at %s  (Alt=%s, Az=%s)\n", e.Kind, e.Time.Format("15:04:05 MST"), e.Altitude.DMSString(0), e.Azimuth.DMSString(0))
	}
}
