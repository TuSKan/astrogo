// Package main propagates 1 Ceres from its real published heliocentric
// osculating orbital elements — resolved live from JPL SBDB, no SPK
// kernel needed — via ephemeris/kepler's two-body Keplerian propagator,
// then feeds the resulting Observable straight into the same
// rise/transit/set machinery every other target in this library uses.
//
// This showcase demonstrates:
//   - catalog/sbdb decoding a resolve.Target's real osculating elements
//     (HasElements/SemiMajorAxis/Eccentricity/.../Epoch) directly from
//     SBDB's own orbit.elements payload — no manual Horizons ELEMENTS
//     query or unit conversion needed
//   - eph.NewFromElements / plan.NewAsteroidFromElements: a full
//     Observable built from those six numbers and an epoch, nothing else
//   - Position/magnitude drifting away from a kernel-backed ephemeris as
//     |t - Epoch| grows, since planetary perturbations aren't modeled
//   - plan.VisibilityEvents working on a Kepler-propagated body exactly
//     like it would on any SPK-kernel-backed or catalog-resolved target
//
// Run: go run ./examples/22_kepler_propagator/
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/catalog"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
)

func main() {
	// Resolve 1 Ceres's real, current osculating elements live from JPL
	// SBDB — catalog/sbdb decodes orbit.elements (e, a, i, om, w, ma) and
	// orbit.epoch directly, natively in AU/degrees, no unit conversion.
	resolver := catalog.NewResolver(catalog.SBDB)

	target, err := resolver.Resolve(context.Background(), "1")
	if err != nil {
		log.Fatalf("resolve 1 Ceres: %v", err)
	}

	if !target.HasElements {
		log.Fatalf("SBDB response for %q carried no orbital elements", target.Name)
	}

	el := eph.Elements{
		Epoch:         target.Epoch,
		SemiMajorAxis: target.SemiMajorAxis,
		Eccentricity:  target.Eccentricity,
		Inclination:   target.Inclination,
		AscendingNode: target.AscendingNode,
		ArgPeriapsis:  target.ArgPeriapsis,
		MeanAnomaly:   target.MeanAnomaly,
	}
	epoch := el.Epoch

	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println("  1 CERES — two-body Keplerian propagation from published elements")
	fmt.Println("  AstroGo | ephemeris/kepler | elements resolved live from JPL SBDB")
	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("── Osculating elements (JPL SBDB, epoch below) ─────────────────────")
	fmt.Println()
	fmt.Printf("  Epoch            %s TDB\n", epoch.Format("2006-01-02 15:04:05"))
	fmt.Printf("  Semi-major axis  %.6f AU\n", el.SemiMajorAxis)
	fmt.Printf("  Eccentricity     %.6f\n", el.Eccentricity)
	fmt.Printf("  Inclination      %s\n", el.Inclination.DMSString(1))
	fmt.Printf("  Ascending node   %s\n", el.AscendingNode.DMSString(1))
	fmt.Printf("  Arg. periapsis   %s\n", el.ArgPeriapsis.DMSString(1))
	fmt.Printf("  Mean anomaly     %s (at epoch)\n", el.MeanAnomaly.DMSString(1))

	if !target.HasH {
		log.Fatalf("SBDB response for %q carried no H/G photometric parameters", target.Name)
	}

	// Note what does NOT happen here: no remote.EnableDownloads for a
	// kernel, no de440s (~32 MB) download — eph.NewFromElements's default
	// base provider is pure analytical SOFA (Sun/Moon/planets), so once
	// the elements above are in hand, everything from here runs offline.
	asteroid, err := plan.NewAsteroidFromElements(target.Name, el, plan.WithHG(target.H, target.G))
	if err != nil {
		log.Fatalf("new asteroid from elements: %v", err)
	}

	// ── Part 1: position/magnitude drift across the year ────────────────────
	fmt.Println()
	fmt.Println("── Position & magnitude across 2026 (two-body propagation) ────────")
	fmt.Println()
	fmt.Println("  Date          RA             Dec           Δ (AU)   Mag")
	fmt.Println("  ──────────    ───────────    ──────────    ──────   ────")

	for m := range 12 {
		t := epoch.AddDays(float64(m) * 30)

		pos, err := asteroid.Position(t)
		if err != nil {
			log.Fatalf("position: %v", err)
		}

		vec, err := asteroid.GeocentricVec(t)
		if err != nil {
			log.Fatalf("geocentric vec: %v", err)
		}

		mag, err := asteroid.ApparentMagnitude(t)
		if err != nil {
			log.Fatalf("apparent magnitude: %v", err)
		}

		fmt.Printf("  %s    %s    %s    %5.2f   %5.2f\n",
			t.Format("2006-01-02"), pos.RA().HMSString(1), pos.Dec().DMSString(0), vec.Norm(), mag)
	}

	fmt.Println()
	fmt.Println("  Two-body propagation ignores planetary perturbations by design —")
	fmt.Println("  accuracy drifts away from Epoch (arcseconds within days, arcminutes")
	fmt.Println("  within months for a main-belt body). For perturbation-aware")
	fmt.Println("  accuracy over long spans, use a real SPK kernel instead:")
	fmt.Println("  eph.NewProvider(ctx, eph.SmallBody, \"1\") — see ephemeris/kepler's")
	fmt.Println("  package doc for the full scope/accuracy discussion.")

	// ── Part 2: it's a full Observable — rise/transit/set works unchanged ──
	fmt.Println()
	fmt.Println("── Rise / Transit / Set at Mauna Kea (epoch night) ─────────────────")
	fmt.Println()

	site, err := plan.NewKnownSite("Mauna Kea")
	if err != nil {
		log.Fatalf("site: %v", err)
	}

	events, err := plan.VisibilityEvents(epoch, epoch.AddDays(1), asteroid, site)
	if err != nil {
		log.Fatalf("visibility events: %v", err)
	}

	if len(events) == 0 {
		fmt.Println("  (not observable above the horizon this night from Mauna Kea)")
	}

	for _, e := range events {
		fmt.Printf("  %-8s  %s   Alt=%s  Az=%s\n",
			e.Kind, e.Time.Format("2006-01-02 15:04 MST"), e.Altitude.DMSString(0), e.Azimuth.DMSString(0))
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println("  A Kepler-propagated body is a plain Observable — VisibilityEvents,")
	fmt.Println("  ObservableWindows, and GetDetails all work exactly as they would")
	fmt.Println("  for any SPK-kernel-backed or catalog-resolved target.")
	fmt.Println("═══════════════════════════════════════════════════════════════════")
}
