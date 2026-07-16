package catalog_test

import (
	"testing"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/plan"
	astrot "github.com/TuSKan/astrogo/time"
)

// TestIntegration demonstrates how to use the catalog system
// to resolve a celestial target from a remote provider and seamlessly
// pass its coordinates into observing planner capabilities.
func TestIntegration(t *testing.T) {
	// 2. Perform a live query to resolve the target mathematically
	// We'll search for the Andromeda Galaxy (M31)
	resolver := catalog.NewResolver(catalog.SIMBAD)

	andromeda, err := resolver.Resolve("M31")
	if err != nil {
		t.Skipf("Skipping integration test — cannot reach SIMBAD: %v", err)
	}

	t.Logf("Resolved Target: %s via %s", andromeda.Name, andromeda.Catalog)
	t.Logf("Coordinates (ICRS): %s\n", andromeda.Coord)

	// 3. Integrate resolved catalog data into observational computations
	// Let's create an Observatory on Earth (e.g. at Mauna Kea)
	loc, _ := coord.NewGeodetic(angle.Deg(-155.4681), angle.Deg(19.8206), 4205.0)

	obs, err := plan.NewSite("Mauna Kea", loc, nil)
	if err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	// We calculate at a specific time (e.g. 2026-04-06 00:00:00 UTC)
	obsTime := astrot.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC)

	// Compute target's altitude and azimuth properties from the Observatory at that time
	ctx := coord.NewContext(obsTime, obs.Location(), atmosphere.StandardAtmosphere)

	altaz, err := ctx.ICRSToAltAz(andromeda.Coord)
	if err != nil {
		t.Fatalf("Transform error: %v", err)
	}

	t.Logf("At %v (UTC), from %s:", obsTime, obs.Name())
	t.Logf("M31 Altitude: %.4f°", altaz.Alt().Degrees())
	t.Logf("M31 Azimuth:  %.4f°", altaz.Az().Degrees())
}
