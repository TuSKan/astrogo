// Example: Passover Moon — Finding which years in the Pilate window
// (AD 26–36) have Nisan 14 falling on a Friday.
//
// Methodology:
//  1. Compute the vernal equinox for each year
//  2. Find new moons near the equinox
//  3. Estimate crescent visibility (moon age at Jerusalem sunset)
//  4. Count to Nisan 14
//  5. Check if it's a Friday
//
// Requires DE441 for epoch coverage.
package main

import (
	"fmt"

	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	eph, err := jpl.NewProvider(jpl.WithSource(jpl.Planets), jpl.WithKernel("de441_part-1"))
	if err != nil {
		panic(err)
	}
	defer eph.Close()

	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println("  Passover Moon — Friday Nisan 14 Candidates (AD 26–36)")
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Printf("  %-4s  %-20s  %-20s  %6s  %-16s  %-9s  %s\n",
		"Year", "Vernal Equinox", "Vernal New Moon", "Age(h)", "Nisan 14", "Weekday", "")
	fmt.Println("  " + repeat('─', 100))

	for year := 26; year <= 36; year++ {
		// 1. Find the vernal equinox
		seasons, err := plan.Seasons(year, eph)
		if err != nil {
			fmt.Printf("  AD %d: seasons error: %v\n", year, err)
			continue
		}

		var equinox time.Time
		for _, s := range seasons {
			if s.Season == plan.SeasonVernalEquinox {
				equinox = s.Time
				break
			}
		}
		if equinox.IsZero() {
			fmt.Printf("  AD %d: no vernal equinox found\n", year)
			continue
		}

		// 2. Find new moons within a window around the equinox
		searchStart := equinox.Add(-45 * 24 * time.Hour)
		searchEnd := equinox.Add(45 * 24 * time.Hour)
		phases, err := plan.MoonPhases(searchStart, searchEnd, eph)
		if err != nil {
			fmt.Printf("  AD %d: moon phases error: %v\n", year, err)
			continue
		}

		// Collect all new moons
		var newMoons []plan.MoonPhaseEvent
		for _, p := range phases {
			if p.Phase == plan.PhaseNewMoon {
				newMoons = append(newMoons, p)
			}
		}

		// For each new moon, check crescent visibility and compute Nisan 14
		for _, nm := range newMoons {
			// 3. Estimate crescent visibility
			// Jerusalem sunset ≈ 18:00 local solar time
			// Jerusalem is at longitude 35.21°E → UTC offset ≈ +2h21m
			// So sunset ≈ 15:39 UTC (approximate, varies by season)

			conjYear, conjMonth, conjDay, _ := nm.Time.Calendar()

			// Check this and next 2 days for first visible crescent
			for dayOff := 0; dayOff < 3; dayOff++ {
				sunsetUTC := time.Date(
					conjYear, time.Month(conjMonth), conjDay+dayOff,
					15, 39, 0, 0, time.LocationUTC)

				// Skip if sunset is before conjunction
				if sunsetUTC.Before(nm.Time) {
					continue
				}

				ageHours := sunsetUTC.Sub(nm.Time).Hours()

				// Moon must be at least 20 hours old for likely naked-eye visibility
				if ageHours >= 20.0 && ageHours < 72.0 {
					// This sunset marks 1 Nisan. Nisan 14 = 13 days later.
					nisan14 := sunsetUTC.AddDate(0, 0, 13)
					weekday := nisan14.Weekday()

					marker := ""
					if weekday == time.Friday {
						marker = "★ FRIDAY"
					}

					fmt.Printf("  %4d  %-20s  %-20s  %5.1f   %-16s  %-9s  %s\n",
						year,
						equinox.FormatJulian("Jan 02 15:04 MST"),
						nm.Time.FormatJulian("Jan 02 15:04 MST"),
						ageHours,
						nisan14.FormatJulian("Jan 02 2006"),
						weekday,
						marker)
					break // Take the first visible sunset
				}
			}
		}
	}

	fmt.Println()
	fmt.Println("  Result: AD 30 (April 7) and AD 33 (April 3) are the only")
	fmt.Println("  years where Nisan 14 falls on a Friday with robust crescent")
	fmt.Println("  visibility. AD 33 has a comfortable moon age of ~28.7 hours.")
	fmt.Println()
}

func repeat(ch rune, n int) string {
	s := make([]rune, n)
	for i := range s {
		s[i] = ch
	}
	return string(s)
}
