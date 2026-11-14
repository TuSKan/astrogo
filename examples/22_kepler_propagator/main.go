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
	"github.com/TuSKan/astrogo/examples/internal/show"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"

	// The kernel-backed sources register their backend rather than being
	// imported by the root package, so a build that wants them says so (#112).
	_ "github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/unit"
)

func main() {
	// Past the SBDB query this example downloads nothing, so it uses no
	// Earth-orientation data either (UT1 = UTC, no polar motion). That can
	// move a rise or set by up to about a second, below the minute printed.
	// For full accuracy, blank-import remote/eop and consent to the IERS
	// bulletin, as 09_geometry_events does.
	time.RegisterModel(time.ZeroModel{})

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

	show.Title("1 Ceres: two-body propagation from published elements",
		"plan.FromCatalog, with the elements resolved live from JPL SBDB")

	// SBDB states the epoch as a TDB Julian Date. Format always prints the
	// UTC clock, which runs about 69 s behind TDB.
	show.Section("Osculating elements")
	show.Fields(
		show.KV("Epoch", "JD %.1f TDB (%s)", target.Epoch.TDB().JD(), target.Epoch.Format("2006-01-02 15:04:05 UTC")),
		show.KV("Semi-major axis", "%.6f AU", target.SemiMajorAxis.AU()),
		show.KV("Eccentricity", "%.6f", target.Eccentricity),
		show.KV("Inclination", "%s", target.Inclination.DMSString(1)),
		show.KV("Ascending node", "%s", target.AscendingNode.DMSString(1)),
		show.KV("Argument of periapsis", "%s", target.ArgPeriapsis.DMSString(1)),
		show.KV("Mean anomaly", "%s at the epoch", target.MeanAnomaly.DMSString(1)),
	)

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
	year := show.NewTable(show.Left("Date"), show.Left("RA"), show.Left("Dec"),
		show.Right("Distance"), show.Right("Magnitude"))

	for m := range 12 {
		t := epoch.Add(unit.Days(float64(m) * 30))

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

		year.Row(t.Format("2006-01-02"), pos.RA().HMSString(1), pos.Dec().DMSString(0),
			fmt.Sprintf("%.2f AU", vec.Norm()), fmt.Sprintf("%.2f", mag))
	}

	show.Section("A year from the epoch")
	year.Print()
	fmt.Println()
	show.Note("Two-body propagation ignores planetary perturbations by design:",
		"its accuracy drifts away from the epoch, by arcseconds within days and",
		"arcminutes within months for a main-belt body. For perturbation-aware",
		"accuracy over long spans, pass a kernel-backed provider to FromCatalog",
		`instead, such as eph.NewProvider(ctx, eph.SmallBody, "1"). See`,
		"ephemeris/kepler's package doc for the whole discussion, and",
		"plan.WithSmallBodyKernels for the same choice inside VisibleTonight.")

	// ── Part 2: it's a full Observable — rise/transit/set works unchanged ──
	site, err := plan.NewKnownSite("Mauna Kea")
	if err != nil {
		log.Fatalf("site: %v", err)
	}

	events, err := plan.VisibilityEvents(epoch, epoch.Add(unit.Days(1)), asteroid, site)
	if err != nil {
		log.Fatalf("visibility events: %v", err)
	}

	show.Section("Rise, transit and set at Mauna Kea, the epoch night")

	if len(events) == 0 {
		show.Note("Not above the horizon this night from Mauna Kea.")
	} else {
		tab := show.NewTable(show.Left("Event"), show.Left("Time"), show.Right("Altitude"), show.Right("Azimuth"))
		for _, e := range events {
			tab.Row(e.Kind.String(), e.Time.Format("2006-01-02 15:04 MST"),
				e.Altitude.DMSString(0), e.Azimuth.DMSString(0))
		}

		tab.Print()
	}

	fmt.Println()
	show.Note("A Kepler-propagated body is a plain Observable: VisibilityEvents,",
		"ObservableWindows and GetDetails work on it exactly as they would for",
		"any kernel-backed or catalog-resolved target.")
}
