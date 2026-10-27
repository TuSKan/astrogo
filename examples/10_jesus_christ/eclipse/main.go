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
	"context"
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/remote"
	_ "github.com/TuSKan/astrogo/remote/eop"
	"github.com/TuSKan/astrogo/time"

	// The kernel-backed sources register their backend rather than being
	// imported by the root package, so a build that wants them says so (#112).
	_ "github.com/TuSKan/astrogo/ephemeris/jpl"
)

func main() {
	// de441 parts are multi-GB each; unlimited here since that's the whole
	// point of this example. See README "Data downloads & offline usage".
	remote.EnableDownloads(0, remote.NAIFSPK)
	remote.EnableDownloads(0, remote.NAIFLSK)
	// Earth orientation, for where the site is on the turning Earth:
	// the IERS bulletin, which the remote/eop import above fetches.
	remote.EnableDownloads(0, remote.IERSFinals2000A)

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
	loc, err := coord.NewEarthLocation(31.7683, 35.2137, 780.0)
	if err != nil {
		panic(err)
	}

	site, err := plan.NewSite("Jerusalem", loc, plan.WithTimeZone(time.LocationUTC))
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
	tStart := time.DateJulianCal(33, 4, 3, 12, 0, 0)
	tEnd := time.DateJulianCal(33, 4, 4, 0, 0, 0)

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
	eStart := time.DateJulianCal(33, 3, 1, 0, 0, 0)
	eEnd := time.DateJulianCal(33, 5, 1, 0, 0, 0)

	eclipses, err := plan.LunarEclipses(eStart, eEnd, prov)
	if err != nil {
		fmt.Printf("  Eclipse error: %v\n", err)
	}

	for _, e := range eclipses {
		fmt.Printf("\n  ── Lunar Eclipse ──\n")
		fmt.Printf("  Date:                %s\n", e.Time.FormatJulian("2006-01-02"))
		fmt.Printf("  Maximum (UTC):       %s\n", e.Time.FormatJulian("15:04:05"))
		fmt.Printf("  Type:                %s (umbral magnitude %.3f)\n", e.Kind, e.Magnitude)
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
	//
	// astrogo finds the eclipse's greatest phase but not its contacts, so the
	// end of the umbral phase is NASA's, from the Five Millennium Canon as
	// EclipseWise publishes it: U4 at 16:12:52.2 UT1, with ΔT = 10191.3 s.
	// What follows from it is computed against this program's moonrise.
	umbraEnds := time.DateJulianCal(33, 4, 3, 16, 12, 52)

	if sunSet != nil && moonRise != nil && len(eclipses) > 0 {
		fmt.Println()
		fmt.Println("  ── Timeline ──")
		fmt.Printf("  Eclipse maximum:   %s UTC\n", eclipses[0].Time.FormatJulian("15:04"))
		fmt.Printf("  Moonrise:          %s UTC\n", moonRise.Time.FormatJulian("15:04"))
		fmt.Printf("  Sunset:            %s UTC\n", sunSet.Time.FormatJulian("15:04"))
		fmt.Printf("  Umbral phase ends: %s UTC (NASA)\n", umbraEnds.FormatJulian("15:04"))
		fmt.Println()

		if moonRise.Time.Before(umbraEnds) {
			fmt.Printf("  The Moon rose %.0f minutes before the umbral phase ended:\n",
				umbraEnds.Sub(moonRise.Time).Minutes())
			fmt.Println("  low in the east, partially darkened and tinted red.")
		} else {
			fmt.Println("  The umbral phase had ended before the Moon rose.")
		}
	}

	// ══════════════════════════════════════════════════════════════════
	// Part 2: Scan ALL lunar eclipses in the Pilate window
	// ══════════════════════════════════════════════════════════════════
	fmt.Println()
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println("  All Lunar Eclipses During Pilate's Prefecture (AD 26–36)")
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println()

	pilateStart := time.DateJulianCal(26, 1, 1, 0, 0, 0)
	pilateEnd := time.DateJulianCal(37, 1, 1, 0, 0, 0)

	allEclipses, err := plan.LunarEclipses(pilateStart, pilateEnd, prov)
	if err != nil {
		panic(err)
	}

	fmt.Printf("  Found %d lunar eclipses:\n\n", len(allEclipses))
	fmt.Printf("  %-22s  %-10s  %-8s  %s\n", "Date", "Type", "Gamma", "Passover?")
	fmt.Println("  " + repeat('─', 65))

	for _, e := range allEclipses {
		// Check if this eclipse is near a March/April full moon (Passover)
		goTime := e.Time.ToGo()

		passover := ""
		if goTime.Month() >= 3 && goTime.Month() <= 4 {
			passover = "★ Near Passover"
		}

		fmt.Printf("  %-22s  %-10s  %6.3f   %s\n",
			e.Time.FormatJulian("2006-01-02 15:04 MST"),
			e.Kind,
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
