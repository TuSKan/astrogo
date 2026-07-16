// Package main demonstrates geometry event detection.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	fmt.Println("=== AstroGo Geometry Event Solver Demonstration ===")

	start := time.NowUTC()
	end := start.AddDays(365) // Scan over an entire year

	// JPL kernel downloads are opt-in — see README "Data downloads &
	// offline usage". de442 is ~115 MB; naif0012.tls (leap seconds) ~5 KB.
	remote.EnableDownloads(remote.NAIFSPK, 200<<20)
	remote.EnableDownloads(remote.NAIFLSK, 0)

	// 1. Setup high-precision JPL Ephemeris (DE442) as requested
	prov, err := eph.NewProvider(context.Background(), eph.Planets, "de442")
	if err != nil {
		log.Fatalf("failed to load jpl de442: %v", err)
	}
	defer func() {
		err := prov.Close()
		if err != nil {
			log.Printf("failed to close provider: %v", err)
		}
	}()

	mars := plan.NewMars(prov)
	venus := plan.NewVenus(prov)
	jupiter := plan.NewJupiter(prov)
	saturn := plan.NewSaturn(prov)
	sun := plan.NewSun(prov)
	moon := plan.NewMoon(prov)

	// 3. Setup Observatory (São Paulo, Brazil with precise coordinates from user's app)
	// Geocentric Events (like Full Moon syzygy) do not depend on the observer to occur,
	// but we can map the generated event timestamp against an observer to see if the event is visible locally!
	loc, _ := coord.NewEarthLocation(-23.5505, -46.6333, 760) // São Paulo
	site, _ := plan.NewSite("São Paulo", loc)

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
	// Lunar Eclipse Detection
	// Uses MoonPhases + ecliptic latitude filter to find actual eclipse candidates.
	fmt.Println("\nLooking for Lunar Eclipses (Next 365 Days):")

	lunarEclipses, err := plan.LunarEclipses(start, end, prov)
	if err != nil {
		log.Fatalf("failed to find lunar eclipses: %v", err)
	}

	if len(lunarEclipses) == 0 {
		fmt.Println("No lunar eclipses found in this time period.")
	}

	for i, ecl := range lunarEclipses {
		// Evaluate if the Moon is actually visible from São Paulo at eclipse time
		altCheck := plan.Altitude{Threshold: angle.Zero()}
		res, _ := altCheck.Check(moon, ecl.Time, site)

		visibilityStr := "Invisible (below horizon)"
		if res.Pass {
			visibilityStr = "Visible!"
		}

		fmt.Printf("[%d] %s at %s  β=%.3f°  γ=%.2f  (%s)\n",
			i+1, ecl.Type, ecl.Time.Format(time.RFC3339),
			ecl.EclipticLatitude.Degrees(), ecl.Gamma, visibilityStr)
	}

	// ----------------------------------------------------
	// Solar Eclipse Detection
	fmt.Println("\nLooking for Solar Eclipses (Next 365 Days):")

	solarEclipses, err := plan.SolarEclipses(start, end, prov)
	if err != nil {
		log.Fatalf("failed to find solar eclipses: %v", err)
	}

	if len(solarEclipses) == 0 {
		fmt.Println("No solar eclipses found in this time period.")
	}

	for i, ecl := range solarEclipses {
		altCheck := plan.Altitude{Threshold: angle.Zero()}
		res, _ := altCheck.Check(moon, ecl.Time, site)

		visibilityStr := "Invisible (below horizon)"
		if res.Pass {
			visibilityStr = "Visible!"
		}

		fmt.Printf("[%d] %s at %s  β=%.3f°  γ=%.2f  (%s)\n",
			i+1, ecl.Type, ecl.Time.Format(time.RFC3339),
			ecl.EclipticLatitude.Degrees(), ecl.Gamma, visibilityStr)
	}

	// ----------------------------------------------------
	// Earth's Apsides (Perihelion & Aphelion)
	fmt.Println("\nEarth's Apsides for current year:")

	apsides, err := plan.Apsides(start.Year(), prov)
	if err != nil {
		log.Fatalf("failed to compute apsides: %v", err)
	}

	for _, a := range apsides {
		fmt.Printf("  %s: %s  (%.6f AU)\n", a.Apsis, a.Time.Format(time.RFC3339), a.Distance)
	}
}
