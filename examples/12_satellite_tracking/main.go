package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	gotime "time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/norad"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris/satellite"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

// This example demonstrates astrogo's NORAD satellite tracking capabilities:
//
//  1. Fetch live GP data from CelestTrak (ISS / NORAD Cat# 25544)
//  2. SGP4 orbit propagation to compute current position
//  3. Sub-satellite point (ground track)
//  4. Pass prediction over an observer location
//
// CelestTrak API: https://celestrak.org/NORAD/elements/gp.php
// Space Data Standards: https://spacedatastandards.org

func main() {
	// ═══════════════════════════════════════════════════════════════════════
	// 1. Fetch ISS GP Data from CelestTrak
	// ═══════════════════════════════════════════════════════════════════════
	header("Fetching ISS GP Data")

	ctx, cancel := context.WithTimeout(context.Background(), 30*gotime.Second)
	defer cancel()

	provider := norad.New()
	gp, err := provider.FetchByID(ctx, 25544) // ISS NORAD catalog number
	if err != nil {
		log.Fatalf("Failed to fetch ISS data: %v", err)
	}

	epoch, _ := gp.EpochTime()
	fmt.Printf("  Name:          %s\n", gp.ObjectName)
	fmt.Printf("  NORAD Cat#:    %d\n", gp.NoradCatID)
	fmt.Printf("  Intl Des:      %s\n", gp.ObjectID)
	fmt.Printf("  Epoch:         %s\n", gp.Epoch)
	fmt.Printf("  Inclination:   %.4f°\n", gp.Inclination)
	fmt.Printf("  Eccentricity:  %.7f\n", gp.Eccentricity)
	fmt.Printf("  Mean Motion:   %.8f rev/day\n", gp.MeanMotion)
	fmt.Printf("  BSTAR:         %.10f\n", gp.BStar)

	// ═══════════════════════════════════════════════════════════════════════
	// 2. Initialize SGP4 Propagator
	// ═══════════════════════════════════════════════════════════════════════
	header("SGP4 Propagation")

	sat, err := satellite.NewFromGP(gp)
	if err != nil {
		log.Fatalf("Failed to create satellite: %v", err)
	}

	fmt.Printf("  Orbital period: %.2f min\n", sat.OrbitalPeriod())

	// Propagate at TLE epoch.
	pos, vel, err := sat.PropagateECI(epoch)
	if err != nil {
		log.Fatalf("Propagation failed: %v", err)
	}
	fmt.Printf("  ECI Position:  (%.2f, %.2f, %.2f) km\n", pos.X, pos.Y, pos.Z)
	fmt.Printf("  ECI Velocity:  (%.4f, %.4f, %.4f) km/s\n", vel.X, vel.Y, vel.Z)
	fmt.Printf("  |r| = %.2f km  |v| = %.4f km/s\n", pos.Norm(), vel.Norm())

	// ═══════════════════════════════════════════════════════════════════════
	// 3. Sub-Satellite Point (Ground Track)
	// ═══════════════════════════════════════════════════════════════════════
	header("Ground Track at Epoch")

	geo, err := sat.SubSatellitePoint(epoch)
	if err != nil {
		log.Fatalf("SubSatellitePoint failed: %v", err)
	}

	alt := geo.Height() / 1e3
	fmt.Printf("  Latitude:   %s (%.4f°)\n", geo.Lat(), geo.Lat().Degrees())
	fmt.Printf("  Longitude:  %s (%.4f°)\n", geo.Lon(), geo.Lon().Degrees())
	fmt.Printf("  Altitude:   %.1f km\n", alt)

	// ═══════════════════════════════════════════════════════════════════════
	// 4. Pass Prediction
	// ═══════════════════════════════════════════════════════════════════════
	header("Pass Prediction — São Paulo, Brazil")

	// Observer: São Paulo (-23.5505°, -46.6333°, 760m)
	observer, err := coord.NewGeodetic(angle.Deg(-46.6333), angle.Deg(-23.5505), 760)
	if err != nil {
		log.Fatalf("NewGeodetic failed: %v", err)
	}

	// Search for passes in the next 24 hours, minimum 10° elevation.
	start := epoch
	end := epoch.AddDays(1.0)
	minEl := angle.Deg(10.0)

	passes, err := plan.SatellitePasses(sat, start, end, observer, minEl)
	if err != nil {
		log.Fatalf("SatellitePasses failed: %v", err)
	}

	if len(passes) == 0 {
		fmt.Println("  No passes above 10° found in the next 24 hours.")
	} else {
		fmt.Printf("  Found %d pass(es) above 10° in 24 hours:\n\n", len(passes))

		for i, pass := range passes {
			fmt.Printf("  ┌─── Pass %d ──────────────────────────────────────────────┐\n", i+1)
			printPassEvent("AOS (Rise)", pass.Rise)
			printPassEvent("TCA (Max) ", pass.Culmination)
			printPassEvent("LOS (Set) ", pass.Set)
			fmt.Printf("  │  Duration:    %s\n", fmtDuration(pass.Duration))
			fmt.Println("  └──────────────────────────────────────────────────────────┘")
			if i < len(passes)-1 {
				fmt.Println()
			}
		}
	}

	// ═══════════════════════════════════════════════════════════════════════
	// 5. Current Look Angle
	// ═══════════════════════════════════════════════════════════════════════
	header("Look Angle at Epoch")

	az, el, rng, err := sat.LookAngle(epoch, observer)
	if err != nil {
		log.Fatalf("LookAngle failed: %v", err)
	}

	fmt.Printf("  Azimuth:     %s (%.2f°)\n", az, az.Degrees())
	fmt.Printf("  Elevation:   %s (%.2f°)\n", el, el.Degrees())
	fmt.Printf("  Range:       %.1f km\n", rng)

	if el.Degrees() > 0 {
		fmt.Printf("  Status:      ☀ ABOVE HORIZON\n")
	} else {
		fmt.Printf("  Status:      ● Below horizon\n")
	}
}

// ── Formatting helpers ────────────────────────────────────────────────────────

func header(title string) {
	width := 62
	pad := width - len(title) - 4
	if pad < 0 {
		pad = 0
	}
	fmt.Printf("\n  ══ %s %s\n\n", title, strings.Repeat("═", pad))
}

func printPassEvent(label string, ev plan.PassEvent) {
	fmt.Printf("  │  %-10s  %s  Az: %6.1f°  El: %5.1f°  Rng: %6.0f km\n",
		label,
		fmtTime(ev.Time),
		ev.Azimuth.Degrees(),
		ev.Elevation.Degrees(),
		ev.Range,
	)
}

func fmtTime(t time.Time) string {
	return t.ToGo().Format("15:04:05")
}

func fmtDuration(d time.Duration) string {
	total := int(d.Seconds())
	min := total / 60
	sec := total % 60
	return fmt.Sprintf("%dm %02ds", min, sec)
}
