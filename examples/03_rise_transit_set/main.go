package main

import (
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/simbad"
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

	// 2. Set a Fixed Target
	sirius, ok := simbad.New().Resolve("Sirius")
	if !ok {
		log.Fatalf("Failed to resolve target: %s", "Sirius")
	}
	target := plan.NewFixed(sirius)

	// 3. Define the Time interval (next 24 hours starting from 7 PM tonight)
	tz, _ := time.LoadLocation("America/Sao_Paulo")
	start := time.Date(2026, 4, 6, 19, 0, 0, 0, tz)
	end := start.Add(24 * time.Hour)

	// 4. Set up an Event Finder (searching every 15 minutes, with 1s tolerance)
	finder := plan.NewEventFinder(15*time.Minute, 1*time.Second)

	// 5. Find Events (crossing over -0.56 degree standard geometric horizon threshold)
	// Real-world astronomical tools use -0.56° (or -34 arcminutes) to approximate standard visual rising/setting.
	events, err := finder.FindEvents(target, start, end, site, angle.Deg(-0.5667))
	if err != nil {
		fmt.Printf("Error finding events: %v\n", err)
		return
	}

	fmt.Printf("Events for %s from %s over 24 hours:\n\n", target.Name(), site.Name())

	for _, e := range events {
		fmt.Printf("- %-10s at %s  (Alt=%s, Az=%s)\n", e.Kind, e.Time.ToGo().In(tz).Format("15:04:05 BRT"), e.Altitude.DMSString(0), e.Azimuth.DMSString(0))
	}
}
