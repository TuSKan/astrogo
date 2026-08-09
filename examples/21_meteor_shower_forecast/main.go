// Package main forecasts a meteor shower's radiant motion, activity
// window, and predicted hourly observed rate at a real observing site —
// using plan.MeteorShower's IMO-standard model, converted from a "how
// many will I see" ZHR figure through the same sky-brightness/limiting-
// magnitude machinery examples/18_sky_brightness demonstrates for point
// targets.
//
// This showcase demonstrates:
//   - Real solar-longitude-based activity window (not a calendar date range)
//   - Radiant drift across the shower's active period
//   - ZHR-to-predicted-observed-rate conversion via ObservedRate
//   - An hourly rate forecast for the 2026 Perseids peak night at Paranal
//
// Run: go run ./examples/21_meteor_shower_forecast/
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/skybrightness/natural"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	// JPL kernel downloads are opt-in — see README "Data downloads &
	// offline usage". de440s is ~32 MB; naif0012.tls (leap seconds) ~5 KB.
	remote.EnableDownloads(remote.NAIFSPK, 200<<20)
	remote.EnableDownloads(remote.NAIFLSK, 0)

	prov, err := eph.NewProvider(context.Background(), eph.Planets, "de440s")
	if err != nil {
		log.Fatalf("ephemeris: %v", err)
	}
	defer func() {
		if err := prov.Close(); err != nil {
			log.Printf("failed to close provider: %v", err)
		}
	}()

	site, err := plan.NewKnownSite("Paranal")
	if err != nil {
		log.Fatalf("site: %v", err)
	}

	shower, err := plan.NewMeteorShower("Perseids")
	if err != nil {
		log.Fatalf("shower: %v", err)
	}

	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Printf("  %s (%s) FORECAST — 2026 apparition\n", shower.Name, shower.Code)
	fmt.Println("  AstroGo | JPL DE440s | Paranal Observatory")
	fmt.Println("═══════════════════════════════════════════════════════════════════")

	// ── Part 1: Activity window & radiant drift ─────────────────────────────
	fmt.Println()
	fmt.Println("── Activity Window & Radiant Drift (2026) ─────────────────────────")
	fmt.Println()
	fmt.Println("  Date        Active   Radiant RA     Radiant Dec")
	fmt.Println("  ──────────  ──────   ────────────   ───────────")

	sweepStart := time.Date(2026, time.July, 10, 0, 0, 0, 0, time.LocationUTC)

	for d := range 46 {
		day := sweepStart.Add(time.Duration(d) * 24 * time.Hour)

		active, err := shower.IsActive(day, prov)
		if err != nil {
			log.Fatalf("active: %v", err)
		}

		ra, dec, err := shower.RadiantAt(day, prov)
		if err != nil {
			log.Fatalf("radiant: %v", err)
		}

		mark := " "
		if active {
			mark = "●"
		}

		fmt.Printf("  %s      %s     %s   %s\n",
			day.Format("2006-01-02"), mark, ra.HMSString(1), dec.DMSString(0))
	}

	// ── Part 2: Hourly predicted rate on the peak night ─────────────────────
	fmt.Println()
	fmt.Println("── Predicted Hourly Rate — Peak Night at Paranal ──────────────────")
	fmt.Println()

	// The IMO's own peak solar longitude (λ☉=140°) falls within a day of
	// August 12 in any given year — RadiantAt/IsActive/ObservedRate below
	// all key off the Sun's real computed longitude, not this calendar
	// date, so it only needs to land inside the shower's active window,
	// not on the exact hour of peak.
	peakNightNoon := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.LocationUTC)
	nextNoon := peakNightNoon.Add(24 * time.Hour)

	dawn, dusk, err := plan.AstronomicalDawnDusk(peakNightNoon, nextNoon, site, prov)
	if err != nil {
		log.Fatalf("twilight: %v", err)
	}

	if dawn == nil || dusk == nil {
		log.Fatal("no astronomical night found in this window")
	}

	// Sky Brightness V2 (docs/skybrightness.md): ModeFast runs only the
	// two fast, simplified components (constant airglow + Krisciunas &
	// Schaefer scattered moonlight) — the same physics v1's
	// CompositeModel(Airglow, ZodiacalLight, Moonlight) ran, minus
	// zodiacal light (Phase 2 scope; not yet re-implemented), re-expressed
	// against the new spectral Engine/Component API. This is a brand-new
	// type, not a v1 compatibility shim — see §15.
	sky, err := natural.NewFastEngine(natural.FastConfig{Ephemeris: prov})
	if err != nil {
		log.Fatalf("sky engine: %v", err)
	}

	constraint := plan.LimitingMagnitudeConstraint{
		Engine: sky, Passband: natural.TopHatJohnsonV(),
		Conversion: skybrightness.NewSchaeferNELM(),
	}

	tz, err := time.LoadLocation("America/Santiago")
	if err != nil {
		log.Fatalf("timezone: %v", err)
	}

	fmt.Println("  Note: the Perseid radiant sits at Dec +58° — from Paranal's -24.6°")
	fmt.Println("  latitude it never climbs far above the horizon, so ObservedRate")
	fmt.Println("  correctly predicts a low rate for most of the night. This is real")
	fmt.Println("  geometry, not a bug: the Perseids are a genuinely poor target for")
	fmt.Println("  southern-hemisphere observers, unlike a shower with a southerly")
	fmt.Println("  radiant (e.g. the Southern Delta Aquariids or Eta Aquariids).")
	fmt.Println()
	fmt.Println("  Local Time (CLT)   Radiant Alt   Predicted rate")
	fmt.Println("  ────────────────   ───────────   ──────────────")

	for tt := dusk.Time; tt.Before(dawn.Time); tt = tt.Add(time.Hour) {
		ra, dec, err := shower.RadiantAt(tt, prov)
		if err != nil {
			log.Fatalf("radiant: %v", err)
		}

		ctx := coord.NewContext(tt, site.Location(), site.Atmosphere())

		aa, err := ctx.ICRSToAltAz(coord.NewICRS(ra, dec))
		if err != nil {
			log.Fatalf("altaz: %v", err)
		}

		rate, err := shower.ObservedRate(tt, site, prov, constraint)
		if err != nil {
			log.Fatalf("observed rate: %v", err)
		}

		fmt.Printf("  %s              %6.1f°       %5.1f/hour\n",
			tt.In(tz).Format("15:04"), aa.Alt().Degrees(), rate)
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Printf("  ZHR (%.0f) is the rate under ideal conditions: radiant at zenith,\n", shower.ZHR)
	fmt.Println("  limiting magnitude 6.5. The predicted rate above instead accounts")
	fmt.Println("  for the radiant's real altitude and the site's real sky brightness")
	fmt.Println("  at each hour of the night — always ≤ ZHR.")
	fmt.Println("═══════════════════════════════════════════════════════════════════")
}
