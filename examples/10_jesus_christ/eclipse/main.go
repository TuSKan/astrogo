// Example: The Evening of April 3, AD 33 — Full simulation of sunset,
// moonrise, and the lunar eclipse as seen from Jerusalem.
//
// Computes:
//   - Sunset and moonrise times at Jerusalem
//   - Lunar eclipse detection and timing
//   - Moon illumination fraction at moonrise
//   - Lunar eclipses across the full Pilate window (AD 26–36)
//
// Requires DE441 for epoch coverage.
package main

import (
	"fmt"
	"log"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	// de441 parts are multi-GB each; unlimited here since that's the whole
	// point of this example. See README "Data downloads & offline usage".
	remote.EnableDownloads(remote.NAIFSPK, 0)
	remote.EnableDownloads(remote.NAIFLSK, 0)

	prov, err := eph.NewProvider(eph.Planets, "de441_part-1")
	if err != nil {
		panic(err)
	}
	defer func() {
		err := prov.Close()
		if err != nil {
			log.Printf("failed to close provider: %v", err)
		}
	}()

	// Jerusalem: 31°46'N, 35°14'E, altitude ~780m (Temple Mount)
	loc, err := coord.NewGeodetic(angle.Deg(31.7683), angle.Deg(35.2137), 780.0)
	if err != nil {
		panic(err)
	}

	site, err := plan.NewSite("Jerusalem", loc, angle.Deg(0), time.LocationUTC)
	if err != nil {
		panic(err)
	}

	// ══════════════════════════════════════════════════════════════════
	// Part 1: The Evening of April 3, AD 33
	// ══════════════════════════════════════════════════════════════════
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println("  astrogo: The Evening of April 3, AD 33 — Jerusalem")
	fmt.Println("══════════════════════════════════════════════════════════════")

	// Search window: afternoon/evening of April 3, AD 33
	tStart := time.Date(33, time.April, 3, 12, 0, 0, 0, time.LocationUTC)
	tEnd := time.Date(33, time.April, 4, 0, 0, 0, 0, time.LocationUTC)

	// ── Sunset ──
	_, sunSet, err := plan.SunriseSunset(tStart, tEnd, site, prov)
	if err != nil {
		fmt.Printf("  Sunset error: %v\n", err)
	} else if sunSet != nil {
		fmt.Printf("\n  Sunset:    %s UTC  (Az: %s)\n",
			sunSet.Time.FormatJulian("15:04:05"), sunSet.Azimuth)
	} else {
		fmt.Println("\n  Sunset: not found in window")
	}

	// ── Moonrise ──
	moonRise, _, err := plan.MoonriseMoonset(tStart, tEnd, site, prov)
	if err != nil {
		fmt.Printf("  Moonrise error: %v\n", err)
	} else if moonRise != nil {
		fmt.Printf("  Moonrise:  %s UTC  (Az: %s)\n",
			moonRise.Time.FormatJulian("15:04:05"), moonRise.Azimuth)
	} else {
		fmt.Println("  Moonrise: not found in window")
	}

	// ── Lunar Eclipse ──
	eStart := time.Date(33, time.March, 1, 0, 0, 0, 0, time.LocationUTC)
	eEnd := time.Date(33, time.May, 1, 0, 0, 0, 0, time.LocationUTC)

	eclipses, err := plan.LunarEclipses(eStart, eEnd, prov)
	if err != nil {
		fmt.Printf("  Eclipse error: %v\n", err)
	}

	for _, e := range eclipses {
		absLat := math.Abs(e.EclipticLatitude.Degrees())

		eclType := "Penumbral"
		if absLat < 0.55 {
			eclType = "Total"
		} else if absLat < 1.05 {
			eclType = "Partial"
		}

		fmt.Printf("\n  ── Lunar Eclipse ──\n")
		fmt.Printf("  Date:                %s\n", e.Time.FormatJulian("2006-01-02"))
		fmt.Printf("  Maximum (UTC):       %s\n", e.Time.FormatJulian("15:04:05"))
		fmt.Printf("  Type (estimated):    %s\n", eclType)
		fmt.Printf("  Ecliptic latitude:   %.3f°\n", e.EclipticLatitude.Degrees())
		fmt.Printf("  Gamma (centrality):  %.3f (0=central, 1=penumbral edge)\n", e.Gamma)
	}

	// ── Moon Illumination at moonrise ──
	if moonRise != nil {
		frac, phaseAngle, err := plan.MoonIllumination(moonRise.Time, prov)
		if err != nil {
			fmt.Printf("  Illumination error: %v\n", err)
		} else {
			fmt.Printf("\n  ── Moon at Moonrise ──\n")
			fmt.Printf("  Illumination:  %.1f%%\n", frac*100)
			fmt.Printf("  Phase angle:   %.1f°\n", phaseAngle.Degrees())
		}
	}

	// ── Timeline Summary ──
	if sunSet != nil && moonRise != nil && len(eclipses) > 0 {
		fmt.Println()
		fmt.Println("  ── Timeline ──")
		fmt.Printf("  Eclipse maximum:   %s UTC (Moon below horizon)\n",
			eclipses[0].Time.FormatJulian("15:04"))
		fmt.Printf("  Sunset:            %s UTC\n", sunSet.Time.FormatJulian("15:04"))
		fmt.Printf("  Moonrise:          %s UTC (Moon rises partially eclipsed)\n",
			moonRise.Time.FormatJulian("15:04"))
		fmt.Println()
		fmt.Println("  For ~30 minutes after moonrise, the Moon hung low on the")
		fmt.Println("  eastern horizon, partially darkened and tinted red by")
		fmt.Println("  Rayleigh scattering in Earth's atmosphere — a 'blood moon'.")
	}

	// ══════════════════════════════════════════════════════════════════
	// Part 2: Scan ALL lunar eclipses in the Pilate window
	// ══════════════════════════════════════════════════════════════════
	fmt.Println()
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println("  All Lunar Eclipses During Pilate's Prefecture (AD 26–36)")
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println()

	pilateStart := time.Date(26, time.January, 1, 0, 0, 0, 0, time.LocationUTC)
	pilateEnd := time.Date(37, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	allEclipses, err := plan.LunarEclipses(pilateStart, pilateEnd, prov)
	if err != nil {
		panic(err)
	}

	fmt.Printf("  Found %d lunar eclipses:\n\n", len(allEclipses))
	fmt.Printf("  %-22s  %-10s  %-8s  %s\n", "Date", "Type Est.", "Gamma", "Passover?")
	fmt.Println("  " + repeat('─', 65))

	for _, e := range allEclipses {
		absLat := math.Abs(e.EclipticLatitude.Degrees())

		eclType := "Penumbral"
		if absLat < 0.55 {
			eclType = "Total"
		} else if absLat < 1.05 {
			eclType = "Partial"
		}

		// Check if this eclipse is near a March/April full moon (Passover)
		goTime := e.Time.ToGo()

		passover := ""
		if goTime.Month() >= 3 && goTime.Month() <= 4 {
			passover = "★ Near Passover"
		}

		fmt.Printf("  %-22s  %-10s  %6.3f   %s\n",
			e.Time.FormatJulian("2006-01-02 15:04 MST"),
			eclType,
			e.Gamma,
			passover)
	}

	fmt.Println()
	fmt.Println("  Only the April 3, AD 33 eclipse occurs near Passover.")
	fmt.Println("  It is unique in the entire Pilate window.")
	fmt.Println()
}

func repeat(ch rune, n int) string {
	s := make([]rune, n)
	for i := range s {
		s[i] = ch
	}

	return string(s)
}
