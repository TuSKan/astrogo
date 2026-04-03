package earth_test

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/earth"
)

func ExampleNewGeodetic() {
	// Create a site at Longitude 0, Latitude 51.4778 (Greenwich)
	lon := angle.Deg(0)
	lat := angle.Deg(51.4778)
	alt := 0.0 // meters

	loc, err := earth.NewGeodetic(lon, lat, alt)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	fmt.Printf("Lat: %.4f, Lon: %.4f\n", loc.Lat.Degrees(), loc.Lon.Degrees())
	// Output: Lat: 51.4778, Lon: 0.0000
}

func ExampleGeodetic_ToECEF() {
	loc, _ := earth.NewGeodetic(angle.Deg(0), angle.Deg(0), 0)
	ecef := loc.ToECEF(earth.WGS84())

	// At (0,0,0) on WGS84, X should be the equatorial radius
	fmt.Printf("X: %.0f km\n", ecef.X/1000)
	// Output: X: 6378 km
}
