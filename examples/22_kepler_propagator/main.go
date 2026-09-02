// Package main resolves 1 Ceres's real published heliocentric osculating
// orbital elements live from JPL SBDB, then hands the resulting
// resolve.Target straight to plan.FromCatalog with no provider — the
// standard entry point every catalog-resolved target goes through.
// FromCatalog itself builds the two-body Keplerian propagator
// (ephemeris/kepler) from those elements, no SPK kernel needed, and the
// result feeds into the same rise/transit/set machinery every other
// target in this library uses.
//
// This showcase demonstrates:
//   - catalog/sbdb decoding a resolve.Target's real osculating elements
//     (HasElements/SemiMajorAxis/Eccentricity/.../Epoch) directly from
//     SBDB's own orbit.elements payload — no manual Horizons ELEMENTS
//     query or unit conversion needed
//   - plan.FromCatalog(target, nil) building a Kepler-propagated
//     *plan.Asteroid automatically whenever HasElements is true and no
//     provider is supplied — "Kepler as the default" for a small body
//     with published elements, the same call any other FromCatalog user
//     already makes
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

	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println("  1 CERES — two-body Keplerian propagation from published elements")
	fmt.Println("  AstroGo | plan.FromCatalog | elements resolved live from JPL SBDB")
	fmt.Println("═══════════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("── Osculating elements (JPL SBDB, epoch below) ─────────────────────")
	fmt.Println()
	fmt.Printf("  Epoch            %s TDB\n", target.Epoch.Format("2006-01-02 15:04:05"))
	fmt.Printf("  Semi-major axis  %.6f AU\n", target.SemiMajorAxis)
	fmt.Printf("  Eccentricity     %.6f\n", target.Eccentricity)
	fmt.Printf("  Inclination      %s\n", target.Inclination.DMSString(1))
	fmt.Printf("  Ascending node   %s\n", target.AscendingNode.DMSString(1))
	fmt.Printf("  Arg. periapsis   %s\n", target.ArgPeriapsis.DMSString(1))
	fmt.Printf("  Mean anomaly     %s (at epoch)\n", target.MeanAnomaly.DMSString(1))

	// Note what does NOT happen here: no remote.EnableDownloads for a
	// kernel, no de440s (~32 MB) download, no manual eph.NewElements/
	// eph.NewFromElements call — FromCatalog(target, nil) sees
	// target.HasElements, builds the Kepler-propagated provider itself,
	// and returns a fully-formed *plan.Asteroid. Everything from here
	// runs entirely offline.
	obs, err := plan.FromCatalog(target, nil)
	if err != nil {
		log.Fatalf("FromCatalog: %v", err)
	}

	asteroid, ok := obs.(*plan.Asteroid)
	if !ok {
		log.Fatalf("FromCatalog did not build a Kepler-propagated *plan.Asteroid for %q (got %T)", target.Name, obs)
	}

	epoch := target.Epoch

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
	fmt.Println("  accuracy over long spans, pass a real SPK-kernel-backed provider")
	fmt.Println("  to FromCatalog instead — e.g. eph.NewProvider(ctx, eph.SmallBody,")
	fmt.Println("  \"1\") — see ephemeris/kepler's package doc for the full")
	fmt.Println("  scope/accuracy discussion, and plan.WithSmallBodyKernels for the")
	fmt.Println("  equivalent override inside plan.VisibleTonight.")

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
