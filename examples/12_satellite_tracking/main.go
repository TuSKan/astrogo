// Package main demonstrates real-time satellite tracking.
package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

// This example demonstrates astrogo's unified Provider API for satellite tracking:
//
//  1. Resolve satellite from catalog (NORAD → CelestTrak)
//  2. SGP4 orbit propagation via Provider.State() — same as JPL planets
//  3. Provider-agnostic altitude
//  4. Topocentric look angle via coord.Context (same API for ISS (Zarya) and Mars)
//  5. Pass prediction
//
// CelestTrak API: https://celestrak.org/NORAD/elements/gp.php

func main() {
	// ═══════════════════════════════════════════════════════════════════════
	// 1. Resolve ISS (Zarya) from NORAD Catalog
	// ═══════════════════════════════════════════════════════════════════════
	header("Resolving ISS (Zarya) from NORAD Catalog")

	resolver := catalog.NewResolver(catalog.NORAD)

	target, err := resolver.Resolve(context.Background(), "ISS (Zarya)")
	if err != nil {
		log.Fatalf("Failed to resolve ISS (Zarya): %v", err)
	}

	fmt.Printf("  Name:     %s\n", target.Name)
	fmt.Printf("  NORAD ID: %s\n", target.ID)
	fmt.Printf("  Intl Des: %s\n", target.Designation)
	fmt.Printf("  Epoch:    %s\n", target.Epoch)

	// ═══════════════════════════════════════════════════════════════════════
	// 2. Create Provider from Target TLE
	// ═══════════════════════════════════════════════════════════════════════
	header("SGP4 Propagation (via Provider.State)")

	prov, err := eph.NewProvider(context.Background(), eph.Satellites, target.Name,
		eph.WithTLE(target.TLELine1, target.TLELine2))
	if err != nil {
		log.Fatalf("Failed to create satellite provider: %v", err)
	}
	defer func() {
		err := prov.Close()
		if err != nil {
			log.Printf("failed to close provider: %v", err)
		}
	}()

	// Use the unified State API — same as for JPL planets.
	epoch := target.Epoch

	state, err := prov.State(0, epoch)
	if err != nil {
		log.Fatalf("State failed: %v", err)
	}

	// Derive RA/Dec from GCRS position.
	icrs, _ := eph.ToICRS(state.Pos)
	fmt.Printf("  RA:       %s\n", icrs.RA())
	fmt.Printf("  Dec:      %s\n", icrs.Dec())
	fmt.Printf("  Distance: %.1f km\n", state.DistanceKm())

	// ═══════════════════════════════════════════════════════════════════════
	// 3. Provider-Agnostic Altitude
	// ═══════════════════════════════════════════════════════════════════════
	header("Orbital Altitude")

	alt, err := eph.Altitude(prov, 0, epoch)
	if err != nil {
		log.Fatalf("Altitude failed: %v", err)
	}

	fmt.Printf("  Altitude: %.1f km above Earth surface\n", alt)

	// ═══════════════════════════════════════════════════════════════════════
	// 4. Look Angle via coord.Context (same API for ISS (Zarya) and Mars)
	// ═══════════════════════════════════════════════════════════════════════
	header("Look Angle — São Paulo, Brazil")

	// Observer: São Paulo (-23.5505°, -46.6333°, 760m)
	observer, err := coord.NewEarthLocation(-23.5505, -46.6333, 760)
	if err != nil {
		log.Fatalf("NewEarthLocation failed: %v", err)
	}

	// Build the observation context (caches SOFA matrices for this time+site).
	ctx := coord.NewContext(epoch, observer, atmosphere.Atmosphere{})

	altaz, err := plan.LookAngle(prov, 0, ctx)
	if err != nil {
		log.Fatalf("LookAngle failed: %v", err)
	}

	fmt.Printf("  Azimuth:    %s (%.2f°)\n", altaz.Az(), altaz.Az().Degrees())
	fmt.Printf("  Elevation:  %s (%.2f°)\n", altaz.Alt(), altaz.Alt().Degrees())
	fmt.Printf("  Range:      %.1f km\n", altaz.Dist())

	if altaz.Alt().Degrees() > 0 {
		fmt.Printf("  Status:     ☀ ABOVE HORIZON\n")
	} else {
		fmt.Printf("  Status:     ● Below horizon\n")
	}

	// ═══════════════════════════════════════════════════════════════════════
	// 5. Pass Prediction
	// ═══════════════════════════════════════════════════════════════════════
	start := epoch
	end := epoch.AddDays(1.0)
	minEl := angle.Deg(20.0)
	loc, _ := time.LoadLocation("America/Sao_Paulo")

	header("Pass Prediction (next 24h, min %1.f° elevation)", minEl.Degrees())

	passes, err := plan.SatellitePasses(prov, target.Name, start, end, observer, minEl)
	if err != nil {
		log.Fatalf("SatellitePasses failed: %v", err)
	}

	if len(passes) == 0 {
		fmt.Printf("  No passes above %.1f° found in the next 24 hours.\n", minEl.Degrees())
	} else {
		fmt.Printf("  Found %d pass(es) above %.1f° in 24 hours:\n\n", len(passes), minEl.Degrees())

		for i, pass := range passes {
			fmt.Printf("  ┌─── Pass %d ──────────────────────────────────────────────┐\n", i+1)
			printPassEvent("AOS (Rise)", pass.Rise, loc)
			printPassEvent("TCA (Max) ", pass.Culmination, loc)
			printPassEvent("LOS (Set) ", pass.Set, loc)
			fmt.Printf("  │  Duration:    %s\n", fmtDuration(pass.Duration))
			fmt.Println("  └──────────────────────────────────────────────────────────┘")

			if i < len(passes)-1 {
				fmt.Println()
			}
		}
	}
}

// ── Formatting helpers ────────────────────────────────────────────────────────

func header(titleFormat string, a ...any) {
	title := fmt.Sprintf(titleFormat, a...)
	width := 62
	pad := max(width-len(title)-4, 0)
	fmt.Printf("\n  ══ %s %s\n\n", title, strings.Repeat("═", pad))
}

func printPassEvent(label string, ev plan.PassEvent, loc *time.Location) {
	fmt.Printf("  │  %-10s  %s  Az: %6.1f°  El: %5.1f°  Rng: %6.0f km\n",
		label,
		fmtTime(ev.Time, loc),
		ev.Azimuth.Degrees(),
		ev.Elevation.Degrees(),
		ev.Range,
	)
}

func fmtTime(t time.Time, loc *time.Location) string {
	return t.ToGo().In(loc).Format("2006-01-02 15:04:05 MST")
}

func fmtDuration(d time.Duration) string {
	total := int(d.Seconds())
	mins := total / 60
	sec := total % 60

	return fmt.Sprintf("%dm %02ds", mins, sec)
}
