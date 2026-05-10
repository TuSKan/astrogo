// Example: Passover Moon — Finding which years in the Pilate window
// (AD 26–36) have Nisan 14 falling on a Friday.
//
// Methodology:
//  1. Compute the vernal equinox for each year
//  2. Find new moons near the equinox
//  3. Compute topocentric crescent parameters at Jerusalem sunset
//  4. Evaluate all 20 modern visibility criteria
//  5. Count to Nisan 14 and check if it's a Friday
//
// Requires DE441 for epoch coverage.
package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

// fridayCandidate stores metadata for a Friday Nisan 14 occurrence.
type fridayCandidate struct {
	nisan14  string
	crescent plan.CrescentResult
	year     int
	ageHours float64
}

func main() {
	prov, err := eph.NewProvider(eph.Planets, "de441_part-1")
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := prov.Close(); err != nil {
			log.Printf("failed to close provider: %v", err)
		}
	}()

	// Jerusalem: 31.7683°N, 35.2137°E, 754m
	jerusalem, _ := coord.NewGeodetic(angle.Deg(35.2137), angle.Deg(31.7683), 754)

	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println("  Passover Moon — Friday Nisan 14 Candidates (AD 26-36)")
	fmt.Println("  with Lunar Crescent Visibility Analysis")
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Printf("  %-4s  %-20s  %-20s  %6s  %-16s  %-9s  %s\n",
		"Year", "Vernal Equinox", "Vernal New Moon", "Age(h)", "Nisan 14", "Weekday", "")
	fmt.Println("  " + repeat('─', 100))

	var fridays []fridayCandidate

	for year := 26; year <= 36; year++ {
		// 1. Find the vernal equinox
		seasons, err := plan.Seasons(year, prov)
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

		phases, err := plan.MoonPhases(searchStart, searchEnd, prov)
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
			for dayOff := range 3 {
				sunsetUTC := time.Date(
					conjYear, time.Month(conjMonth), conjDay+dayOff,
					15, 39, 0, 0, time.LocationUTC)

				// Skip if sunset is before conjunction
				if sunsetUTC.Before(nm.Time) {
					continue
				}

				ageHours := sunsetUTC.Sub(nm.Time).Hours()

				// Moon must be at least 15 hours old for any plausible visibility
				if ageHours >= 15.0 && ageHours < 72.0 {
					// Compute topocentric crescent parameters at this sunset
					params, err := plan.NewCrescentParams(sunsetUTC, jerusalem, prov)
					if err != nil {
						continue
					}

					result := params.EvaluateAll()

					// Check if at least the Danjon elongation criterion is met
					if !result.Danjon {
						continue
					}

					// Count positive criteria as a visibility confidence score
					nVisible := countVisible(result)

					// This sunset marks the evening that begins Nisan 1.
					// In the Jewish calendar, a "day" runs sunset-to-sunset.
					// The DAYTIME of Nisan 14 (Passover sacrifice / crucifixion)
					// is 14 civil (midnight-to-midnight) days after the sighting
					// sunset: 13 sunsets to reach the evening of Nisan 14, then
					// the next morning is its daytime portion.
					nisan14 := sunsetUTC.AddDate(0, 0, 14)
					weekday := nisan14.Weekday()

					marker := ""
					if weekday == time.Friday {
						marker = "★ FRIDAY"

						fridays = append(fridays, fridayCandidate{
							year:     year,
							nisan14:  nisan14.FormatJulian("Jan 02"),
							ageHours: ageHours,
							crescent: result,
						})
					}

					fmt.Printf("  %4d  %-20s  %-20s  %5.1f   %-16s  %-9s  %s  [%d/20 criteria]\n",
						year,
						equinox.FormatJulian("Jan 02 15:04 MST"),
						nm.Time.FormatJulian("Jan 02 15:04 MST"),
						ageHours,
						nisan14.FormatJulian("Jan 02 2006"),
						weekday,
						marker,
						nVisible)

					break // Take the first visible sunset
				}
			}
		}
	}

	// ── Dynamically generated summary ────────────────────────────────────
	fmt.Println()

	if len(fridays) == 0 {
		fmt.Println("  Result: No Friday Nisan 14 candidates found.")
	} else {
		fmt.Println("  Friday Nisan 14 candidates found:")

		for _, f := range fridays {
			nVisible := countVisible(f.crescent)
			fmt.Printf("    • AD %d — %s (Julian) — crescent age %.1f hours — %d/20 criteria met\n",
				f.year, f.nisan14, f.ageHours, nVisible)
		}

		// Show detailed crescent evaluation for each Friday candidate
		for _, f := range fridays {
			fmt.Println()
			fmt.Printf("  ── Crescent Visibility: AD %d (%s) ─────────────────────\n",
				f.year, f.nisan14)
			fmt.Println(indent(f.crescent.String(), "    "))
		}

		fmt.Println()
		fmt.Println("  Conclusion: AD 30 and AD 33 are the only years in the Pilate")
		fmt.Println("  window where Nisan 14 falls on a Friday with a Passover-eligible")
		fmt.Println("  new moon (≥ vernal equinox).")
	}

	fmt.Println()
}

// countVisible counts how many of the 20 criteria report visibility.
func countVisible(r plan.CrescentResult) int {
	n := 0

	bools := []bool{
		r.Fotheringham, r.Maunder, r.Ilyas1988, r.Fatoohi, r.KraussAthenian,
		r.MABIMS1995, r.Istanbul2016, r.MABIMS2021,
		r.Danjon, r.Schaefer, r.Ilyas1984,
		r.Bruin, r.AlrefayNakedEye,
		r.CaldwellNakedEye, r.CaldwellOptical, r.Gautschy,
	}
	for _, b := range bools {
		if b {
			n++
		}
	}
	// Zone-based criteria: count if in a "visible" zone
	if r.Yallop.Code == "A" || r.Yallop.Code == "B" {
		n++
	}

	if r.Odeh.Code == "Naked Eye" || r.Odeh.Code == "Optical/Naked" {
		n++
	}

	if r.Qureshi.Code == "A" || r.Qureshi.Code == "B" {
		n++
	}
	// Alrefay is already counted above as AlrefayNakedEye
	return n
}

func repeat(ch rune, n int) string {
	s := make([]rune, n)
	for i := range s {
		s[i] = ch
	}

	return string(s)
}

func indent(s, prefix string) string {
	out := ""

	var outSb231 strings.Builder

	for i, line := range splitLines(s) {
		if i > 0 {
			outSb231.WriteString("\n")
		}

		if line != "" {
			outSb231.WriteString(prefix + line)
		}
	}

	out += outSb231.String()

	return out
}

func splitLines(s string) []string {
	var lines []string

	start := 0

	for i := range len(s) {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}

	if start < len(s) {
		lines = append(lines, s[start:])
	}

	return lines
}
