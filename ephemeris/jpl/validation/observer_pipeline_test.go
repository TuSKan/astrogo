//go:build network

package jpl_test

import (
	"math"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	atime "github.com/TuSKan/astrogo/time"
)

// ensure EOP is loaded before tests
func init() {
	// earth package auto-loads finals2000A in its init, so EOP is fully online.
}

func TestPhase1ObserverPipelineAgainstHorizons(t *testing.T) {
	// The Earth Observer setup
	site, err := coord.NewGeodetic(angle.Deg(51.477), angle.Deg(0.0), 0.0)
	if err != nil {
		t.Fatalf("Failed to create site: %v", err)
	}

	// We test November 1st, 2024 at exactly noon UTC
	tStrStart := "2024-11-01 12:00"
	tStrStop := "2024-11-01 12:01"

	// Create the AstroGo native time
	obsTime := atime.Date(2024, 11, 1, 12, 0, 0, 0, time.UTC)

	// Fetch Topocentric Observer Table for Mars (NAIF ID: 499)
	marsHorizons, err := fetchObserverTable(499, "Mars", site.Lon().Degrees(), site.Lat().Degrees(), site.Height(), tStrStart, tStrStop)
	if err != nil {
		t.Fatalf("Failed to fetch Horizons data: %v", err)
	}

	t.Logf("Horizons Astrometric J2000 : RA=%v, Dec=%v", marsHorizons.AstroRA, marsHorizons.AstroDec)
	t.Logf("Horizons Apparent (True Date) : RA=%v, Dec=%v", marsHorizons.AppRA, marsHorizons.AppDec)
	t.Logf("Horizons Refracted Observer: Azimuth=%v, Elevation=%v", marsHorizons.Azimuth, marsHorizons.Elevation)

	// ---- AstroGo Pipeline Execution ----

	// 1. Inject the Astrometric ICRS position directly from Horizons
	astro := coord.NewICRS(angle.Deg(marsHorizons.AstroRA), angle.Deg(marsHorizons.AstroDec))
	// Note: We bypass feeding pure proper motion / parallax / RV because the Horizons
	// astrometric position natively integrates the offset for the specific target date.
	// If we used J2000.0 catalog coordinates, we would use proper motion here.
	// Wait, Horizons outputs Astrometric coordinates FOR THE EPOCH OF DATE!
	// Actually, Astrometric is defined wrt ICRS cleanly.

	// 2. Map through to Apparent!
	apparent := coord.AstrometricToApparent(coord.NewAstrometric(astro.RA(), astro.Dec()), obsTime)

	// AstroGo uses CIRS (Celestial Intermediate Reference System) for Apparent coords.
	// Horizons uses classical True Equator and Equinox of Date.
	// The offsets are generally within a few arcseconds strictly due to origin frames,
	// so we allow a ~5 arcsecond margin for RA/Dec purely due to convention disparity,
	// though they describe the exact same physical line of sight!
	raDiff := math.Abs(apparent.RA().Degrees() - marsHorizons.AppRA)
	decDiff := math.Abs(apparent.Dec().Degrees() - marsHorizons.AppDec)

	// Ensure wrap-around distance is shortest
	if raDiff > 180.0 {
		raDiff = 360.0 - raDiff
	}

	t.Logf("AstroGo  Apparent (CIRS)      : RA=%v, Dec=%v", apparent.RA().Degrees(), apparent.Dec().Degrees())
	t.Logf("Apparent Deviation -> RA: %.3f arcsec, Dec: %.3f arcsec", raDiff*3600.0, decDiff*3600.0)

	// Note: We DO NOT assert strictly on RA here! The Equation of the Origins separates the
	// Celestial Intermediate Origin (AstroGo CIRS origin) from the True Equinox of Date (Horizons origin)
	// by significant non-linear margins (observed here as >1000 arcseconds!).
	// Declination remains mostly invariant physically.
	if decDiff*3600 > 10.0 {
		t.Errorf("Apparent Dec deviation fundamentally shifted frames: %.3f arcsec", decDiff*3600)
	}

	// 3. Map to Observed Topocentric (Alt/Az)
	// Horizons outputs AIRLESS coordinates unless told otherwise (we specifically stripped REFRACTION flag logic since it breaks output bounds unless specific atmospheric limits are present, which we can't reliably inject).
	// Therefore, we MUST use our RefractionNone model to properly evaluate that Earth Orientation Parameters apply perfectly natively.
	atm := coord.StandardAtmosphere
	atm.Model = coord.RefractionNone{}

	observed := coord.ApparentToObserved(apparent, obsTime, site, atm)

	t.Logf("AstroGo  Geometric Observer  : Azimuth=%v, Elevation=%v", observed.Az().Degrees(), observed.Alt().Degrees())

	// Compare Alt/Az. This is strictly physical and should NOT depend on CI vs equinox mapping.
	// We expect sub-arcsecond precision natively matching JPL's DE Ephemerides.
	azDiff := math.Abs(observed.Az().Degrees() - marsHorizons.Azimuth)
	if azDiff > 180 {
		azDiff = 360.0 - azDiff
	}
	altDiff := math.Abs(observed.Alt().Degrees() - marsHorizons.Elevation)

	t.Logf("Topocentric Deviation -> Azimuth: %.3f arcsec, Elevation: %.3f arcsec", azDiff*3600.0, altDiff*3600.0)

	// Threshold: 1.0 arcsecond (Extremely strict. Allows fractional deviation from Earth Orientation Parameter smoothing)
	if azDiff*3600 > 1.0 {
		t.Errorf("Topocentric Azimuth mathematically deviated from Horizons by %.3f arcsec", azDiff*3600)
	}
	if altDiff*3600 > 1.0 {
		t.Errorf("Topocentric Elevation mathematically deviated from Horizons by %.3f arcsec", altDiff*3600)
	}
}
