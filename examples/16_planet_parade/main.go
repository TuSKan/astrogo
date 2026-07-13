// Package main reconstructs the "Great Planet Parade" of February 28, 2025 — when all
// seven planets were simultaneously above the horizon in the evening sky from São Paulo.
//
// This example demonstrates:
//   - JPL DE442 ephemerides for all 7 planets
//   - USNO-validated rise/set/transit with standard atmospheric refraction
//   - Twilight boundaries (civil, nautical, astronomical)
//   - AltAz positions at any epoch via coord.Context
//   - Ecliptic coordinates for clustering analysis
//   - Appulse (closest approach) detection via plan.Appulses
//
// Run: go run ./examples/16_planet_parade/
package main

import (
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

type planetDef struct {
	Target plan.Observable
	Name   string
}

func main() {
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("  The Great Planet Parade — February 28, 2025")
	fmt.Println("  Seven Planets in the Evening Sky from São Paulo")
	fmt.Println("═══════════════════════════════════════════════════════════════")

	// ── Setup: JPL DE442 ──────────────────────────────────────────────
	// Kernel downloads are opt-in — see README "Data downloads & offline
	// usage". de442 is ~115 MB; naif0012.tls (leap seconds) ~5 KB.
	remote.EnableDownloads(remote.NAIFSPK, 200<<20)
	remote.EnableDownloads(remote.NAIFLSK, 0)

	prov, err := eph.NewProvider(eph.Planets, "de442")
	if err != nil {
		log.Fatalf("failed to load JPL DE442: %v", err)
	}
	defer func() {
		err := prov.Close()
		if err != nil {
			log.Printf("failed to close provider: %v", err)
		}
	}()

	brtz, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		log.Fatalf("failed to load timezone: %v", err)
	}

	loc, _ := coord.NewEarthLocation(-23.5505, -46.6333, 760)
	site, _ := plan.NewSite("São Paulo", loc, angle.Zero(), brtz)
	atm := atmosphere.AtAltitude(760)

	planets := []planetDef{
		{plan.NewMercury(prov), "Mercury"},
		{plan.NewVenus(prov), "Venus"},
		{plan.NewMars(prov), "Mars"},
		{plan.NewJupiter(prov), "Jupiter"},
		{plan.NewSaturn(prov), "Saturn"},
		{plan.NewUranus(prov), "Uranus"},
		{plan.NewNeptune(prov), "Neptune"},
	}

	// ── Part 1: Solar Events ──────────────────────────────────────────
	fmt.Println("\n── Solar Events (February 28, 2025 BRT) ──────────────────────")

	day := time.Date(2025, time.February, 28, 0, 0, 0, 0, brtz)
	next := day.Add(24 * time.Hour)

	_, sunset, err := plan.SunriseSunset(day, next, site, prov)
	if err != nil {
		log.Fatalf("SunriseSunset: %v", err)
	}

	_, civilDusk, err := plan.CivilDawnDusk(day, next, site, prov)
	if err != nil {
		log.Fatalf("CivilDawnDusk: %v", err)
	}

	_, nautDusk, err := plan.NauticalDawnDusk(day, next, site, prov)
	if err != nil {
		log.Fatalf("NauticalDawnDusk: %v", err)
	}

	_, astroDusk, err := plan.AstronomicalDawnDusk(day, next, site, prov)
	if err != nil {
		log.Fatalf("AstronomicalDawnDusk: %v", err)
	}

	fmt.Printf("  Sunset:            %s BRT\n", sunset.Time.In(brtz).Format("15:04:05"))
	fmt.Printf("  Civil dusk:        %s BRT  (Sun < −6°)\n", civilDusk.Time.In(brtz).Format("15:04:05"))
	fmt.Printf("  Nautical dusk:     %s BRT  (Sun < −12°)\n", nautDusk.Time.In(brtz).Format("15:04:05"))
	fmt.Printf("  Astronomical dusk: %s BRT  (Sun < −18°)\n", astroDusk.Time.In(brtz).Format("15:04:05"))

	// ── Part 2: Planetary Positions at Civil Dusk ──────────────────────
	fmt.Println("\n── Planetary Positions at Civil Dusk ──────────────────────────")
	fmt.Printf("  Time: %s UTC\n\n", civilDusk.Time.Format("2006-01-02 15:04:05"))
	fmt.Println("  Planet    │ Ecl. λ   │ Alt      │ Az       │ Airmass")
	fmt.Println("  ──────────┼──────────┼──────────┼──────────┼────────")

	ctx := coord.NewContext(civilDusk.Time, loc, atm)

	for _, p := range planets {
		icrs, err := p.Target.Position(civilDusk.Time)
		if err != nil {
			fmt.Printf("  %-9s │ error: %v\n", p.Name, err)
			continue
		}

		altaz, _ := ctx.ICRSToAltAz(icrs)
		ecl := coord.ICRSToEcliptic(icrs, civilDusk.Time)

		alt := altaz.Alt().Degrees()
		amStr := "below"

		if alt > 0 {
			am, _ := atmosphere.Airmass(altaz.Alt())
			amStr = fmt.Sprintf("%.1f", am)
		}

		fmt.Printf("  %-9s │ %6.1f°  │ %+6.1f°  │ %5.1f°   │ %s\n",
			p.Name, ecl.Lon().Degrees(), alt, altaz.Az().Degrees(), amStr)
	}

	// ── Part 3: Altitude Timeline ────────────────────────────────────
	fmt.Println("\n── Altitude Timeline (sunset → +75 min, every 5 min) ─────────")
	fmt.Print("  Time (BRT) │")

	for _, p := range planets {
		name := p.Name
		if len(name) > 5 {
			name = name[:5]
		}

		fmt.Printf(" %-5s │", name)
	}

	fmt.Println()
	fmt.Print("  ───────────┼")

	for range planets {
		fmt.Print("───────┼")
	}

	fmt.Println()

	tStart := sunset.Time
	for i := range 76 {
		t := tStart.Add(time.Duration(int64(i) * 1 * int64(time.Minute)))
		c := coord.NewContext(t, loc, atm)

		fmt.Printf("  %s │", t.In(brtz).Format("15:04"))

		for _, p := range planets {
			icrs, err := p.Target.Position(t)
			if err != nil {
				fmt.Print("  err  │")
				continue
			}

			altaz, _ := c.ICRSToAltAz(icrs)

			alt := altaz.Alt().Degrees()
			if alt < 0 {
				fmt.Print("  ───  │")
			} else {
				fmt.Printf(" %+4.0f° │", alt)
			}
		}

		fmt.Println()
	}

	// ── Part 4: Ecliptic Longitude Span ──────────────────────────────
	fmt.Println("\n── Ecliptic Clustering Analysis ───────────────────────────────")

	lons := make([]float64, 0, len(planets))

	for _, p := range planets {
		icrs, _ := p.Target.Position(civilDusk.Time)
		ecl := coord.ICRSToEcliptic(icrs, civilDusk.Time)
		lons = append(lons, ecl.Lon().Degrees())
	}

	// Find minimum arc span containing all planets
	minSpan := 360.0

	for i := range lons {
		maxArc := 0.0

		for j := range lons {
			arc := lons[j] - lons[i]
			for arc < 0 {
				arc += 360
			}

			if arc > maxArc {
				maxArc = arc
			}
		}

		if maxArc < minSpan {
			minSpan = maxArc
		}
	}

	fmt.Printf("  Ecliptic longitude span: %.0f°\n", minSpan)

	if minSpan < 180 {
		fmt.Println("  All planets are in the same half of the sky ✓")
	}

	// ── Part 5: Near-Conjunctions ────────────────────────────────────
	fmt.Println("\n── Closest Approaches (Feb 15 – Mar 15, 2025) ────────────────")

	searchStart := time.Date(2025, time.February, 15, 0, 0, 0, 0, time.LocationUTC)
	searchEnd := time.Date(2025, time.March, 15, 0, 0, 0, 0, time.LocationUTC)

	pairs := [][2]int{
		{1, 6}, // Venus–Neptune
		{1, 4}, // Venus–Saturn
		{0, 4}, // Mercury–Saturn
		{3, 5}, // Jupiter–Uranus
		{2, 3}, // Mars–Jupiter
	}

	for _, pair := range pairs {
		appulses, err := plan.Appulses(searchStart, searchEnd,
			planets[pair[0]].Target, planets[pair[1]].Target)
		if err != nil || len(appulses) == 0 {
			continue
		}

		for _, app := range appulses {
			fmt.Printf("  %s – %s: closest %.2f° on %s\n",
				planets[pair[0]].Name, planets[pair[1]].Name,
				app.Value, app.Time.Format("2006-01-02 15:04 MST"))
		}
	}

	// ── Part 6: Visibility Summary ───────────────────────────────────
	fmt.Println("\n── Visibility Summary ─────────────────────────────────────────")
	fmt.Println("  Planet    │ Naked Eye │ Notes")
	fmt.Println("  ──────────┼───────────┼──────────────────────────────────")

	visibility := []struct {
		name, eye, notes string
	}{
		{"Venus", "✅ Yes", "Brilliant, brightest object in the sky"},
		{"Jupiter", "✅ Yes", "Bright, unmissable high in the sky"},
		{"Mars", "✅ Yes", "Steady, reddish, near zenith"},
		{"Saturn", "⚠️  Hard", "Faint and very low, in twilight glow"},
		{"Mercury", "⚠️  Hard", "Bright but very low, ~20 min window"},
		{"Uranus", "❌ No", "Too faint naked-eye — binoculars required"},
		{"Neptune", "❌ No", "Always requires telescope"},
	}
	for _, v := range visibility {
		fmt.Printf("  %-9s │ %-9s │ %s\n", v.name, v.eye, v.notes)
	}

	fmt.Println("\n═══════════════════════════════════════════════════════════════")
	fmt.Println("  Computed with astrogo + JPL DE442 ephemerides")
	fmt.Println("  Rise/set validated to ≤0.6 min vs U.S. Naval Observatory")
	fmt.Println("═══════════════════════════════════════════════════════════════")
}
