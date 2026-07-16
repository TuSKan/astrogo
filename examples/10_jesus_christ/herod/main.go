// Example: Herod's Eclipse — Computing lunar eclipses around 4 BC and 1 BC
// to evaluate candidates for the eclipse mentioned by Josephus.
//
// Includes Jerusalem visibility analysis: whether the Moon was above
// the horizon during the eclipse, and whether it was night or day.
//
// Requires DE441 (part-1) for deep historical epoch coverage.
package main

import (
	"context"
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

	prov, err := eph.NewProvider(context.Background(), eph.Planets, "de441_part-1")
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

	site, err := plan.NewSite("Jerusalem", loc, time.LocationUTC)
	if err != nil {
		panic(err)
	}

	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println("  Herod's Eclipse — Lunar Eclipse Candidates (5 BC – AD 1)")
	fmt.Println("  Observer: Jerusalem (31.77°N, 35.21°E)")
	fmt.Println("══════════════════════════════════════════════════════════════════════")
	fmt.Println()

	// Search for all lunar eclipses from 5 BC to AD 1
	// Astronomical years: 5 BC = -4, 1 BC = 0, AD 1 = 1
	start := time.Date(-4, time.January, 1, 0, 0, 0, 0, time.LocationUTC)
	end := time.Date(2, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	eclipses, err := plan.LunarEclipses(start, end, prov)
	if err != nil {
		panic(err)
	}

	fmt.Printf("  Found %d lunar eclipses in the search window:\n\n", len(eclipses))
	fmt.Printf("  %-22s  %-8s  %-12s  %-10s  %-6s  %-12s  %s\n",
		"Date (Julian, UTC)", "|β| (°)", "Gamma", "Type", "Year", "Visibility", "")
	fmt.Println("  " + repeat('─', 95))

	for _, e := range eclipses {
		absLat := math.Abs(e.EclipticLatitude.Degrees())

		// Classify eclipse type from ecliptic latitude
		eclType := "Penumbral"
		if absLat < 0.55 {
			eclType = "Total"
		} else if absLat < 1.05 {
			eclType = "Partial"
		}

		// Year in historical notation
		year, _, _, _ := e.Time.JulianCalendar()

		yearStr := fmt.Sprintf("AD %d", year)
		if year <= 0 {
			yearStr = fmt.Sprintf("%d BC", 1-year)
		}

		// Check Jerusalem visibility:
		// Is the Sun below and Moon above the horizon at eclipse maximum?
		window := 12 * time.Hour
		dayStart := e.Time.Add(-window)
		dayEnd := e.Time.Add(window)

		var visibility string

		// Check if Sun is set (= night) at eclipse time
		_, sunSet, sunErr := plan.SunriseSunset(dayStart, dayEnd, site, prov)
		sunRise, _, sunErrR := plan.SunriseSunset(dayStart, dayEnd, site, prov)
		_ = sunErrR

		isNight := false

		if sunErr == nil && sunSet != nil {
			// Night = after sunset and before next sunrise
			if e.Time.After(sunSet.Time) {
				isNight = true
			}

			if sunRise != nil && e.Time.Before(sunRise.Time) {
				isNight = true
			}
		}

		// Check if Moon is above horizon at eclipse time
		moonUp := false

		moonRise, moonSet, moonErr := plan.MoonriseMoonset(dayStart, dayEnd, site, prov)
		if moonErr == nil {
			if moonRise != nil && moonSet != nil {
				// Moon is up between rise and set
				if e.Time.After(moonRise.Time) && e.Time.Before(moonSet.Time) {
					moonUp = true
				}
			} else if moonRise != nil && moonSet == nil {
				// Moon rose but hasn't set yet in the window
				if e.Time.After(moonRise.Time) {
					moonUp = true
				}
			} else if moonRise == nil && moonSet != nil {
				// Moon was already up, sets during window
				if e.Time.Before(moonSet.Time) {
					moonUp = true
				}
			}
		}

		if isNight && moonUp {
			visibility = "★ Visible"
		} else if moonUp && !isNight {
			visibility = "  Daytime"
		} else if !moonUp {
			visibility = "  Moon down"
		} else {
			visibility = "  Daytime"
		}

		marker := ""
		if isNight && moonUp && (eclType == "Total" || eclType == "Partial") {
			marker = "◀ CANDIDATE"
		}

		fmt.Printf("  %-22s  %6.3f    %6.3f    %-10s  %-6s  %-12s  %s\n",
			e.Time.FormatJulian("2006-01-02 15:04 MST"),
			absLat,
			e.Gamma,
			eclType,
			yearStr,
			visibility,
			marker)
	}

	fmt.Println()
	fmt.Println("  Only eclipses marked ★ Visible were observable from Jerusalem at night.")
	fmt.Println()
}

func repeat(ch rune, n int) string {
	s := make([]rune, n)
	for i := range s {
		s[i] = ch
	}

	return string(s)
}
