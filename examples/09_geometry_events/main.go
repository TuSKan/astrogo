// Package main demonstrates geometry event detection: conjunctions, greatest
// elongations, eclipses and the Earth's apsides over one calendar year.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/angle"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/remote"
	_ "github.com/TuSKan/astrogo/remote/eop"
	"github.com/TuSKan/astrogo/time"

	// The kernel-backed sources register their backend rather than being
	// imported by the root package, so a build that wants them says so (#112).
	_ "github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/unit"
)

func main() {
	// One calendar year, fixed so the output is the same on every run and
	// can be checked against published almanacs. Use time.NowUTC() as the
	// start to look ahead from today instead.
	const year = 2027

	start := time.Date(year, 1, 1, 0, 0, 0, 0, time.LocationUTC)
	end := time.Date(year+1, 1, 1, 0, 0, 0, 0, time.LocationUTC)

	fmt.Printf("=== Geometry events in %d (UTC) ===\n", year)

	// JPL kernel downloads are opt-in — see README "Data downloads &
	// offline usage". de442 is ~115 MB; naif0012.tls (leap seconds) ~5 KB.
	remote.EnableDownloads(200<<20, remote.NAIFSPK)
	remote.EnableDownloads(0, remote.NAIFLSK)
	// Earth orientation, for where the site is on the turning Earth:
	// the IERS bulletin, which the remote/eop import above fetches.
	remote.EnableDownloads(0, remote.IERSFinals2000A)

	// 1. The JPL DE442 ephemeris
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

	// 2. An observer, for the one event below that depends on where you are:
	// a lunar eclipse is seen wherever the Moon is above the horizon.
	site, err := plan.NewSiteEarthLocation("Quinta Calixto", -22.528478, -46.473002, 835.05)
	if err != nil {
		log.Fatalf("site: %v", err)
	}

	// ----------------------------------------------------
	// Conjunction: Mars and Venus having the same Right Ascension
	fmt.Println("\nConjunctions of Venus and Mars:")

	events, err := plan.Conjunctions(start, end, venus, mars)
	if err != nil {
		log.Fatalf("failed to find conjunctions: %v", err)
	}

	if len(events) == 0 {
		fmt.Println("  none this year")
	}

	for i, e := range events {
		fmt.Printf("  [%d] %s at %s\n", i+1, e.Kind, e.Time.Format("2006-01-02 15:04 MST"))
	}

	// ----------------------------------------------------
	// Greatest Elongation: Venus reaching peak angular distance from the Sun
	fmt.Println("\nGreatest elongations of Venus:")

	elongEvents, err := plan.GreatestElongations(start, end, venus, sun)
	if err != nil {
		log.Fatalf("failed to find elongation events: %v", err)
	}

	if len(elongEvents) == 0 {
		fmt.Println("  none this year")
	}

	for i, e := range elongEvents {
		fmt.Printf("  [%d] %s at %s (%.2f° from the Sun)\n", i+1, e.Kind, e.Time.Format("2006-01-02 15:04 MST"), e.Value)
	}

	// ----------------------------------------------------
	// The Great Conjunction of Jupiter and Saturn comes about every 20 years,
	// so this one search looks further ahead.
	jupSatEnd := start.Add(unit.Days(365.25 * 20))

	fmt.Printf("\nConjunctions of Jupiter and Saturn, %d to %d:\n", year, year+20)

	jupSatEvents, err := plan.Conjunctions(start, jupSatEnd, jupiter, saturn)
	if err != nil {
		log.Fatalf("failed to find Jupiter-Saturn conjunctions: %v", err)
	}

	if len(jupSatEvents) == 0 {
		fmt.Println("  none in this period")
	}

	for i, e := range jupSatEvents {
		fmt.Printf("  [%d] %s at %s\n", i+1, e.Kind, e.Time.Format("2006-01-02 15:04 MST"))
	}

	// ----------------------------------------------------
	// Lunar eclipses, and whether the Moon is up at Quinta Calixto at maximum.
	// Gamma is the canon's: the Moon's distance from the axis of Earth's
	// shadow at maximum, in Earth radii, positive when it passes north of it.
	fmt.Printf("\nLunar eclipses (visibility from %s):\n", site.Name())

	lunarEclipses, err := plan.LunarEclipses(start, end, prov)
	if err != nil {
		log.Fatalf("failed to find lunar eclipses: %v", err)
	}

	if len(lunarEclipses) == 0 {
		fmt.Println("  none this year")
	}

	for i, ecl := range lunarEclipses {
		res, err := plan.Altitude{Threshold: angle.Zero()}.Check(moon, ecl.Time, site)
		if err != nil {
			log.Fatalf("moon altitude: %v", err)
		}

		visibility := "Moon below the horizon at maximum"
		if res.Pass {
			visibility = "Moon up at maximum"
		}

		fmt.Printf("  [%d] %s at %s  gamma %+.4f  (%s)\n",
			i+1, ecl.Type, ecl.Time.Format("2006-01-02 15:04 MST"), ecl.Gamma, visibility)
	}

	// ----------------------------------------------------
	// Solar eclipses. Where one is seen depends on the path of the Moon's
	// shadow across the Earth, which this list does not compute, so it gives
	// only the instant of greatest eclipse.
	fmt.Println("\nSolar eclipses (instant of greatest eclipse):")

	solarEclipses, err := plan.SolarEclipses(start, end, prov)
	if err != nil {
		log.Fatalf("failed to find solar eclipses: %v", err)
	}

	if len(solarEclipses) == 0 {
		fmt.Println("  none this year")
	}

	for i, ecl := range solarEclipses {
		fmt.Printf("  [%d] %s at %s  gamma %+.4f\n",
			i+1, ecl.Type, ecl.Time.Format("2006-01-02 15:04 MST"), ecl.Gamma)
	}

	// ----------------------------------------------------
	// Earth's Apsides (Perihelion & Aphelion)
	fmt.Printf("\nEarth's apsides in %d:\n", year)

	apsides, err := plan.Apsides(year, prov)
	if err != nil {
		log.Fatalf("failed to compute apsides: %v", err)
	}

	for _, a := range apsides {
		fmt.Printf("  %-10s %s  (%.6f AU)\n", a.Apsis, a.Time.Format("2006-01-02 15:04 MST"), a.Distance.AU())
	}
}
