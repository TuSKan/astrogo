package coords

import (
	"math"
	"testing"
	"time"
)

func TestICRSToObserved(t *testing.T) {
	// Let's create a functional test to verify standard integration logic
	loc := Location{
		Latitude:  52.0,
		Longitude: 0.0,
		Elevation: 0.0,
		Pressure:  1013.25,
		Temp:      15.0,
		Humidity:  0.5,
	}

	// Target strictly across the local meridian explicitly.
	// We'll just verify the mathematical pipeline evaluates continuously.
	obsTime := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	raRad := 0.0
	decRad := 52.0 * (math.Pi / 180.0) // Zenith target roughly

	altRad, azRad, err := ICRSToObserved(raRad, decRad, obsTime, loc, 0.0)
	if err != nil {
		t.Fatalf("unexpected mathematical calculation failure natively: %v", err)
	}

	// Ensure numerical stability outputs aren't purely NaN defaults.
	if math.IsNaN(altRad) || math.IsNaN(azRad) {
		t.Fatalf("native SOFA translation calculated invalid geometries")
	}
}
