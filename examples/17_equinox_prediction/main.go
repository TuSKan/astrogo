// Package main predicts all equinoxes, solstices, apsides, moon phases,
// and eclipses for a multi-year span — a complete Earth-Moon-Sun almanac
// computed from first principles using JPL DE442 ephemerides.
//
// This showcase demonstrates:
//   - Season prediction via ecliptic longitude root-finding
//   - Chandrupatla/Brent sub-second refinement
//   - Season duration asymmetry from Earth's orbital eccentricity
//   - Moon phase detection via elongation crossing
//   - Lunar/solar eclipse detection via ecliptic latitude filtering
//   - Topocentric Moon position (diurnal parallax correction)
//
// Run: go run ./examples/17_equinox_prediction/
package main

import (
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	prov, err := eph.NewProvider(eph.Planets, "de442")
	if err != nil {
		log.Fatalf("ephemeris: %v", err)
	}
	defer func() {
		err := prov.Close()
		if err != nil {
			log.Printf("failed to close provider: %v", err)
		}
	}()

	brtz, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		log.Fatalf("timezone: %v", err)
	}

	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println("  EQUINOX & SOLSTICE PREDICTION — 2024–2033")
	fmt.Println("  AstroGo | JPL DE442 | São Paulo (23°33'S, 46°38'W)")
	fmt.Println("═══════════════════════════════════════════════════════════════════")

	// ── Part 1: Seasons for a decade ────────────────────────────────────────
	fmt.Println()
	fmt.Println("── Equinoxes & Solstices (2024–2033 BRT) ──────────────────────────")
	fmt.Println()
	fmt.Println("  Year │ Vernal Equinox       │ Summer Solstice      │ Autumnal Equinox     │ Winter Solstice")
	fmt.Println("  ─────┼──────────────────────┼──────────────────────┼──────────────────────┼──────────────────────")

	for year := 2024; year <= 2033; year++ {
		events, err := plan.Seasons(year, prov)
		if err != nil {
			log.Fatalf("seasons %d: %v", year, err)
		}

		fmt.Printf("  %d │", year)

		for _, e := range events {
			fmt.Printf(" %s │", e.Time.In(brtz).Format("Jan 02 15:04:05"))
		}

		fmt.Println()
	}

	// ── Part 2: Season durations for 2026 ───────────────────────────────────
	fmt.Println()
	fmt.Println("── Season Durations (2026, Northern Hemisphere) ───────────────────")
	fmt.Println()

	events26, _ := plan.Seasons(2026, prov)
	events27, _ := plan.Seasons(2027, prov)

	durations := []struct {
		start time.Time
		end   time.Time
		name  string
	}{
		{events26[0].Time, events26[1].Time, "Spring"},
		{events26[1].Time, events26[2].Time, "Summer"},
		{events26[2].Time, events26[3].Time, "Autumn"},
		{events26[3].Time, events27[0].Time, "Winter"},
	}

	totalDays := 0.0

	for _, d := range durations {
		days := d.end.SubDays(d.start)
		totalDays += days
		hours := (days - float64(int(days))) * 24
		fmt.Printf("  %-8s %6.2f days  (%dd %02dh)\n", d.name, days, int(days), int(hours))
	}

	fmt.Printf("  %-8s %6.2f days  (tropical year)\n", "Total", totalDays)
	fmt.Println()
	fmt.Println("  Note: Northern summer (93.6d) > winter (89.0d) because Earth is")
	fmt.Println("  near aphelion in July — Kepler's 2nd law slows orbital speed.")

	// ── Part 3: Earth's Apsides for 2026 ────────────────────────────────────
	fmt.Println()
	fmt.Println("── Earth's Apsides (2026) ─────────────────────────────────────────")
	fmt.Println()

	apsides, err := plan.Apsides(2026, prov)
	if err != nil {
		log.Fatalf("apsides: %v", err)
	}

	for _, a := range apsides {
		fmt.Printf("  %-12s %s  (%.6f AU)\n",
			a.Apsis, a.Time.In(brtz).Format("Jan 02 15:04:05 MST"), a.Distance)
	}

	eccentricity := (apsides[1].Distance - apsides[0].Distance) / (apsides[1].Distance + apsides[0].Distance)
	fmt.Printf("\n  Orbital eccentricity: e = %.6f\n", eccentricity)

	// ── Part 4: 2026 Moon Phases (first 3 months) ───────────────────────────
	fmt.Println()
	fmt.Println("── Moon Phases: January–March 2026 ────────────────────────────────")
	fmt.Println()

	phStart := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)
	phEnd := time.Date(2026, time.April, 1, 0, 0, 0, 0, time.LocationUTC)

	phases, err := plan.MoonPhases(phStart, phEnd, prov)
	if err != nil {
		log.Fatalf("moon phases: %v", err)
	}

	for _, p := range phases {
		icon := ""

		switch p.Phase {
		case plan.PhaseNewMoon:
			icon = "🌑"
		case plan.PhaseFirstQuarter:
			icon = "🌓"
		case plan.PhaseFullMoon:
			icon = "🌕"
		case plan.PhaseLastQuarter:
			icon = "🌗"
		}

		fmt.Printf("  %s %-15s %s\n", icon, p.Phase, p.Time.In(brtz).Format("Jan 02 15:04 MST"))
	}

	// ── Part 5: 2026 Eclipses ───────────────────────────────────────────────
	fmt.Println()
	fmt.Println("── Eclipses of 2026 ───────────────────────────────────────────────")
	fmt.Println()

	eclStart := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)
	eclEnd := time.Date(2027, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	lunarEcl, _ := plan.LunarEclipses(eclStart, eclEnd, prov)
	solarEcl, _ := plan.SolarEclipses(eclStart, eclEnd, prov)

	for _, e := range lunarEcl {
		classification := "Penumbral"

		absLat := e.EclipticLatitude.Degrees()
		if absLat < 0 {
			absLat = -absLat
		}

		if absLat < 0.55 {
			classification = "Total"
		} else if absLat < 1.05 {
			classification = "Partial"
		}

		fmt.Printf("  🌑 Lunar Eclipse (%s)  %s  |β|=%.3f°  γ=%.3f\n",
			classification, e.Time.In(brtz).Format("Jan 02 15:04 MST"), absLat, e.Gamma)
	}

	for _, e := range solarEcl {
		classification := "Partial"

		absLat := e.EclipticLatitude.Degrees()
		if absLat < 0 {
			absLat = -absLat
		}

		if absLat < 0.99 {
			classification = "Total/Annular"
		}

		fmt.Printf("  🌕 Solar Eclipse (%s)  %s  |β|=%.3f°  γ=%.3f\n",
			classification, e.Time.In(brtz).Format("Jan 02 15:04 MST"), absLat, e.Gamma)
	}

	// ── Part 6: Topocentric Moon (diurnal parallax) ──────────────────────────
	fmt.Println()
	fmt.Println("── Topocentric Moon (São Paulo, Vernal Equinox 2026) ──────────────")
	fmt.Println()

	moonTarget := plan.NewMoon(prov)
	now := events26[0].Time // Use the actual vernal equinox moment
	loc, _ := coord.NewEarthLocation(-23.5505, -46.6333, 760.0)
	site, _ := plan.NewSite("São Paulo", loc, angle.Zero(), brtz)
	atm := atmosphere.AtAltitude(760)
	ctx := coord.NewContext(now, loc, atm)

	details, err := moonTarget.GetDetails(ctx)
	if err != nil {
		log.Fatalf("moon details: %v", err)
	}

	fmt.Println(details)

	// Show moon illumination at equinox
	frac, phaseAngle, _ := plan.MoonIllumination(now, prov)
	fmt.Printf("  Moon illumination: %.1f%% (phase angle: %.1f°)\n", frac*100, phaseAngle.Degrees())

	// Moon rise/set on equinox day
	eqDay := time.Date(2026, time.March, 20, 0, 0, 0, 0, time.LocationUTC)
	eqNext := eqDay.Add(24 * time.Hour)

	moonrise, moonset, _ := plan.MoonriseMoonset(eqDay, eqNext, site, prov)
	if moonrise != nil {
		fmt.Printf("  Moonrise:  %s  Az: %s\n", moonrise.Time.In(brtz).Format("15:04:05 MST"), moonrise.Azimuth.DMSString(0))
	}

	if moonset != nil {
		fmt.Printf("  Moonset:   %s  Az: %s\n", moonset.Time.In(brtz).Format("15:04:05 MST"), moonset.Azimuth.DMSString(0))
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println("  Computed with AstroGo + JPL DE442 ephemerides")
	fmt.Println("  Seasons validated to <1 min vs USNO (41/41 tests)")
	fmt.Println("  Moon diurnal parallax: ~1° correction applied")
	fmt.Println("═══════════════════════════════════════════════════════════════════")
}
