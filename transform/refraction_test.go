package transform_test

import (
	"math"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/earth"
	atime "github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/transform"
)

func TestRefractionModes(t *testing.T) {
	site := earth.Geodetic{
		Lat:    angle.Deg(45.0),
		Lon:    angle.Deg(-90.0),
		Height: 100.0,
	}

	obsTime := atime.Date(2023, 5, 1, 6, 0, 0, 0, time.UTC)
	
	// A star likely to be somewhat low on the horizon
	astro := coord.Astrometric{
		RA:  angle.Deg(180.0),
		Dec: angle.Deg(10.0),
	}

	// 1. No Refraction Model
	atmNone := earth.StandardAtmosphere
	atmNone.Model = earth.RefractionNone{}
	obsNone := transform.AstrometricToObserved(astro, obsTime, site, atmNone)

	// 2. SOFA (Native) Refraction Model
	atmSOFA := earth.StandardAtmosphere
	atmSOFA.Model = earth.RefractionSOFA{}
	obsSOFA := transform.AstrometricToObserved(astro, obsTime, site, atmSOFA)

	// 3. Approximate Refraction Model
	atmApprox := earth.StandardAtmosphere
	atmApprox.Model = earth.RefractionApproximate{}
	obsApprox := transform.AstrometricToObserved(astro, obsTime, site, atmApprox)

	// Assertions
	altNone := obsNone.Alt.Degrees()
	altSOFA := obsSOFA.Alt.Degrees()
	altApprox := obsApprox.Alt.Degrees()

	// Both SOFA and Approx should lift the star (positive refraction)
	if altSOFA <= altNone {
		t.Errorf("SOFA Refraction failed to lift altitude: Geometric=%v, Refracted=%v", altNone, altSOFA)
	}
	if altApprox <= altNone {
		t.Errorf("Approx Refraction failed to lift altitude: Geometric=%v, Refracted=%v", altNone, altApprox)
	}

	// Assuming the star is above 10 degrees, Approx and SOFA should be within 0.2 arcminutes of each other.
	diff := math.Abs(altSOFA - altApprox)
	if diff > (0.2 / 60.0) { // 0.2 arcminutes
		t.Errorf("Approximate refraction deviated from SOFA rigorous model by too much. SOFA=%v, Approx=%v, Diff=%v arcmin", altSOFA, altApprox, diff*60)
	}
	
	// RefractionNone should perfectly bypass internal SOFA refraction yielding identical results 
	// to if pressure=0 was explicitly passed with no interface logic.
	// Since 0.0 pressure completely shuts down Atco13 refraction mathematically.
	if altNone > altSOFA {
		t.Errorf("Disabled refraction still generated lift")
	}
}
