// Package main reconstructs the "Great Planet Parade" of February 28, 2025 — when all
// seven planets were simultaneously above the horizon in the evening sky from Quinta Calixto.
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
	"context"
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/examples/internal/show"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/remote"
	_ "github.com/TuSKan/astrogo/remote/eop"
	"github.com/TuSKan/astrogo/time"

	// The kernel-backed sources register their backend rather than being
	// imported by the root package, so a build that wants them says so (#112).
	_ "github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/unit"
)

type planetDef struct {
	Target plan.Observable
	Name   string
}

func main() {
	show.Title("The great planet parade, February 28, 2025",
		"Seven planets in the evening sky from Quinta Calixto")

	// ── Setup: JPL DE442 ──────────────────────────────────────────────
	// Kernel downloads are opt-in — see README "Data downloads & offline
	// usage". de442 is ~115 MB; naif0012.tls (leap seconds) ~5 KB.
	remote.EnableDownloads(200<<20, remote.NAIFSPK)
	remote.EnableDownloads(0, remote.NAIFLSK)
	// Earth orientation, for where the site is on the turning Earth:
	// the IERS bulletin, which the remote/eop import above fetches.
	remote.EnableDownloads(0, remote.IERSFinals2000A)

	prov, err := eph.NewProvider(context.Background(), eph.Planets, "de442")
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

	site, err := plan.NewSiteEarthLocation("Quinta Calixto", -22.528478, -46.473002, 835.05, plan.WithTimeZone(brtz))
	if err != nil {
		log.Fatalf("site: %v", err)
	}

	loc := site.Location()
	atm := atmosphere.AtAltitude(835.05)

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
	day := time.Date(2025, time.February, 28, 0, 0, 0, 0, brtz)
	next := day.Add(unit.Hours(24))

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

	show.Section("Dusk, February 28, 2025")
	show.Fields(
		show.KV("Sunset", "%s", sunset.Time.In(brtz).Format("15:04:05 MST")),
		show.KV("Civil dusk", "%s  Sun 6° down", civilDusk.Time.In(brtz).Format("15:04:05 MST")),
		show.KV("Nautical dusk", "%s  Sun 12° down", nautDusk.Time.In(brtz).Format("15:04:05 MST")),
		show.KV("Astronomical dusk", "%s  Sun 18° down", astroDusk.Time.In(brtz).Format("15:04:05 MST")),
	)

	// ── Part 2: Planetary Positions at Civil Dusk ──────────────────────
	ctx := coord.NewContext(civilDusk.Time, loc, atm)

	positions := show.NewTable(show.Left("Planet"), show.Right("Ecliptic λ"),
		show.Right("Altitude"), show.Right("Azimuth"), show.Right("Airmass"))

	for _, p := range planets {
		icrs, err := p.Target.Position(civilDusk.Time)
		if err != nil {
			log.Fatalf("%s position: %v", p.Name, err)
		}

		altaz, err := ctx.ICRSToAltAz(icrs)
		if err != nil {
			log.Fatalf("altaz: %v", err)
		}

		ecl := coord.ICRSToEcliptic(icrs, civilDusk.Time)

		alt := altaz.Alt().Degrees()
		amStr := "below"

		if alt > 0 {
			am, err := atmosphere.Airmass(altaz.Alt())
			if err != nil {
				log.Fatalf("am: %v", err)
			}

			amStr = fmt.Sprintf("%.1f", am)
		}

		positions.Row(p.Name, fmt.Sprintf("%.1f°", ecl.Lon().Degrees()),
			fmt.Sprintf("%+.1f°", alt), fmt.Sprintf("%.1f°", altaz.Az().Degrees()), amStr)
	}

	show.Section("The planets at civil dusk, " + civilDusk.Time.In(brtz).Format("15:04 MST"))
	positions.Print()

	// ── Part 3: Altitude Timeline ────────────────────────────────────
	cols := make([]show.Column, 0, 1+len(planets))
	cols = append(cols, show.Left("Time"))

	for _, p := range planets {
		cols = append(cols, show.Right(p.Name))
	}

	timeline := show.NewTable(cols...)

	tStart := sunset.Time
	for minutes := 0; minutes <= 75; minutes += 5 {
		t := tStart.Add(unit.Minutes(float64(minutes)))
		c := coord.NewContext(t, loc, atm)

		row := make([]string, 0, 1+len(planets))
		row = append(row, t.In(brtz).Format("15:04"))

		for _, p := range planets {
			icrs, err := p.Target.Position(t)
			if err != nil {
				log.Fatalf("%s position: %v", p.Name, err)
			}

			altaz, err := c.ICRSToAltAz(icrs)
			if err != nil {
				log.Fatalf("altaz: %v", err)
			}

			if alt := altaz.Alt().Degrees(); alt < 0 {
				row = append(row, "set")
			} else {
				row = append(row, fmt.Sprintf("%+.0f°", alt))
			}
		}

		timeline.Row(row...)
	}

	show.Section("Altitude from sunset, every 5 minutes")
	timeline.Print()

	// ── Part 4: Ecliptic Longitude Span ──────────────────────────────
	lons := make([]float64, 0, len(planets))

	for _, p := range planets {
		icrs, err := p.Target.Position(civilDusk.Time)
		if err != nil {
			log.Fatalf("icrs: %v", err)
		}

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

	half := "no: they spread over more than half of it"
	if minSpan < 180 {
		half = "yes"
	}

	show.Section("How tightly they cluster on the ecliptic")
	show.Fields(
		show.KV("Span in ecliptic longitude", "%.0f°", minSpan),
		show.KV("All in one half of the sky", "%s", half),
	)

	// ── Part 5: Near-Conjunctions ────────────────────────────────────
	searchStart := time.Date(2025, time.February, 15, 0, 0, 0, 0, time.LocationUTC)
	searchEnd := time.Date(2025, time.March, 15, 0, 0, 0, 0, time.LocationUTC)

	pairs := [][2]int{
		{1, 6}, // Venus–Neptune
		{1, 4}, // Venus–Saturn
		{0, 4}, // Mercury–Saturn
		{3, 5}, // Jupiter–Uranus
		{2, 3}, // Mars–Jupiter
	}

	closest := show.NewTable(show.Left("Pair"), show.Right("Separation"), show.Left("When"))

	for _, pair := range pairs {
		a, b := planets[pair[0]], planets[pair[1]]

		appulses, err := plan.Appulses(searchStart, searchEnd, a.Target, b.Target)
		if err != nil {
			log.Fatalf("%s–%s closest approach: %v", a.Name, b.Name, err)
		}

		if len(appulses) == 0 {
			closest.Row(a.Name+" – "+b.Name, "", "none in the window")

			continue
		}

		for _, app := range appulses {
			closest.Row(a.Name+" – "+b.Name, fmt.Sprintf("%.2f°", app.Value), app.Time.Format("2006-01-02 15:04 MST"))
		}
	}

	show.Section("Closest approaches, February 15 to March 15, 2025")
	closest.Print()

	// ── Part 6: Visibility Summary ───────────────────────────────────
	// A judgment from the tables above and the planets' brightness, not a
	// computation: Mercury and Saturn are low in the twilight glow, and
	// Uranus and Neptune are too faint for the naked eye anywhere.
	summary := show.NewTable(show.Left("Planet"), show.Left("Naked eye"), show.Left("Notes"))
	for _, v := range []struct{ name, eye, notes string }{
		{"Venus", "yes", "brilliant, the brightest object in the sky"},
		{"Jupiter", "yes", "bright, 44° up in the north-northwest"},
		{"Mars", "yes", "steady and reddish, 35° up in the north-northeast"},
		{"Saturn", "hard", "faint and very low, in the twilight glow"},
		{"Mercury", "hard", "bright but very low, a window of about 20 minutes"},
		{"Uranus", "no", "too faint for the naked eye: binoculars"},
		{"Neptune", "no", "always needs a telescope"},
	} {
		summary.Row(v.name, v.eye, v.notes)
	}

	show.Section("What could be seen")
	summary.Print()
	fmt.Println()
	show.Note("Computed with astrogo and the JPL DE442 ephemeris. Sun and Moon",
		"rise and set are validated against the U.S. Naval Observatory to",
		"0.6 minutes (docs/VALIDATION.md).")
}
