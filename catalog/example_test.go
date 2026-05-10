package catalog_test

import (
	"fmt"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/plan"
	astrot "github.com/TuSKan/astrogo/time"
)

// Example_integration demonstrates how to use the catalog system
// to resolve a celestial target from a remote provider and seamlessly
// pass its coordinates into observing planner capabilities.
func Example_integration() {
	// 2. Perform a live query to resolve the target mathematically
	// We'll search for the Andromeda Galaxy (M31)
	resolver := catalog.NewResolver(catalog.SIMBAD)

	andromeda, err := resolver.Resolve("M31")
	if err != nil {
		fmt.Println("Failed to resolve target.")
		return
	}

	fmt.Printf("Resolved Target: %s via %s\n", andromeda.Name, andromeda.Catalog)
	fmt.Printf("Coordinates (ICRS): %s\n\n", andromeda.Coord)

	// 3. Integrate resolved catalog data into observational computations
	// Let's create an Observatory on Earth (e.g. at Mauna Kea)
	loc, _ := coord.NewGeodetic(angle.Deg(-155.4681), angle.Deg(19.8206), 4205.0)

	obs, err := plan.NewSite("Mauna Kea", loc, angle.Deg(0), nil)
	if err != nil {
		fmt.Println("Failed to create site:", err)
		return
	}

	// We calculate at a specific time (e.g. 2026-04-06 00:00:00 UTC)
	obsTime := astrot.Date(2026, 4, 6, 0, 0, 0, 0, time.UTC)

	// Compute target's altitude and azimuth properties from the Observatory at that time
	ctx := coord.NewContext(obsTime, obs.Location(), atmosphere.StandardAtmosphere)

	altaz, err := ctx.ICRSToAltAz(andromeda.Coord)
	if err != nil {
		fmt.Println("Transform error:", err)
		return
	}

	fmt.Printf("At %v (UTC), from %s:\n", obsTime, obs.Name())
	fmt.Printf("M31 Altitude: %.4f°\n", altaz.Alt().Degrees())
	fmt.Printf("M31 Azimuth:  %.4f°\n", altaz.Az().Degrees())
}
