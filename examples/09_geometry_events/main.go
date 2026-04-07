package main

import (
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	fmt.Println("=== AstroGo Geometry Event Solver Demonstration ===")

	start := time.NowUTC()
	end := start.AddDays(365) // Scan over an entire year

	// 1. Setup high-precision JPL Ephemeris (DE442) as requested
	eph, err := jpl.NewProvider(jpl.WithSource(jpl.Planets), jpl.WithKernel("de442"))
	if err != nil {
		log.Fatalf("failed to load jpl de442: %v", err)
	}
	defer eph.Close()

	// 2. Defining targets using our high precision provider
	mars := plan.NewBody(ephemeris.Mars, eph)
	venus := plan.NewBody(ephemeris.Venus, eph)
	jupiter := plan.NewBody(ephemeris.Jupiter, eph)
	saturn := plan.NewBody(ephemeris.Saturn, eph)
	sun := plan.NewBody(ephemeris.Sun, eph)
	moon := plan.NewBody(ephemeris.Moon, eph)

	// 3. Setup Observatory (São Paulo, Brazil with precise coordinates from user's app)
	// Geocentric Events (like Full Moon syzygy) do not depend on the observer to occur,
	// but we can map the generated event timestamp against an observer to see if the event is visible locally!
	loc, _ := coord.NewGeodetic(angle.Deg(-46.6525), angle.Deg(-23.600833), 786)
	site, _ := plan.NewSite("São Paulo", loc, angle.Zero(), nil)

	// ----------------------------------------------------
	// Conjunction: Mars and Venus having the same Right Ascension
	fmt.Println("\nLooking for Conjunctions between Mars and Venus (Next 365 Days):")

	events, err := plan.Conjunctions(start, end, venus, mars)
	if err != nil {
		log.Fatalf("failed to find conjunctions: %v", err)
	}

	if len(events) == 0 {
		fmt.Println("No conjunctions found in this time period.")
	}
	for i, e := range events {
		fmt.Printf("[%d] %s at %s\n", i+1, e.Kind, e.Time.Format(time.RFC3339))
	}

	// ----------------------------------------------------
	// Greatest Elongation: Venus reaching peak angular distance from the Sun
	fmt.Println("\nLooking for Greatest Elongations (East and West) of Venus (Next 365 Days):")

	elongEvents, err := plan.GreatestElongations(start, end, venus, sun)
	if err != nil {
		log.Fatalf("failed to find elongation events: %v", err)
	}

	if len(elongEvents) == 0 {
		fmt.Println("No greatest elongation events found in this time period.")
	} else {
		for i, e := range elongEvents {
			fmt.Printf("[%d] %s at %s (Separation: %.2f degrees)\n", i+1, e.Kind, e.Time.Format(time.RFC3339), e.Value)
		}
	}
	// ----------------------------------------------------
	// Jupiter and Saturn Conjunction (The Great Conjunction)
	// We might need to scan further into the future to find one, but let's check!
	jupSatEnd := start.AddDays(365 * 20)
	fmt.Println("\nLooking for Conjunctions between Jupiter and Saturn (Next 20 Years):")

	jupSatEvents, err := plan.Conjunctions(start, jupSatEnd, jupiter, saturn)
	if err != nil {
		log.Fatalf("failed to find Jupiter-Saturn conjunctions: %v", err)
	}

	if len(jupSatEvents) == 0 {
		fmt.Println("No Jupiter-Saturn conjunctions found in this time period.")
	}
	for i, e := range jupSatEvents {
		fmt.Printf("[%d] %s at %s\n", i+1, e.Kind, e.Time.Format(time.RFC3339))
	}

	// ----------------------------------------------------
	// Lunar Eclipse / Full Moon
	// A Lunar Eclipse strictly occurs at the exact moment of Opposition between the Sun and the Moon.
	// This event demonstrates how to trace Syzygy alignments using the convenient wrapper.
	fmt.Println("\nLooking for Full Moons (Sun-Moon Oppositions) leading to possible Lunar Eclipses (Next 365 Days):")

	lunarEvents, err := plan.LunarEclipses(start, end, eph)
	if err != nil {
		log.Fatalf("failed to find Lunar Oppositions: %v", err)
	}

	if len(lunarEvents) == 0 {
		fmt.Println("No Lunar Oppositions found in this time period.")
	}
	// Let's just print the first 5 events so we don't flood the output (there are ~12 full moons a year)
	for i, e := range lunarEvents {
		if i >= 5 {
			fmt.Printf("... plus %d more full moons throughout the year.\n", len(lunarEvents)-5)
			break
		}

		// Evaluate if the Moon is actually visible from São Paulo at the exact moment of Syzygy!
		altCheck := plan.Altitude{Threshold: angle.Zero()}
		res, _ := altCheck.Check(moon, e.Time, site)

		visibilityStr := "Invisible (below horizon)"
		if res.Pass {
			visibilityStr = "Visible!"
		}

		fmt.Printf("[%d] Lunar Opposition (Full Moon) at %s  -  Altitude from SP: %5.2f° (%s)\n",
			i+1, e.Time.Format(time.RFC3339), res.Value, visibilityStr)
	}
}
