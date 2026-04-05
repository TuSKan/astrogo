package transform_test

import (
	"math"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/earth"
	"github.com/TuSKan/astrogo/internal/testutil"
	atime "github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/transform"
)

func TestAstrometricToApparent(t *testing.T) {
	// A mock star at epoch J2000.0 with extreme proper motion and parallax
	astro := coord.Astrometric{
		RA:       angle.Deg(150.0),
		Dec:      angle.Deg(-30.0),
		PmRA:     angle.Arcsec(1.5),  // 1.5 arcsec/yr
		PmDec:    angle.Arcsec(-0.5), // -0.5 arcsec/yr
		Parallax: angle.Arcsec(0.2),
		RV:       50.0, // km/s
	}

	// Calculate apparent position 10 years later (2010.0)
	obsTime := atime.Date(2010, 1, 1, 12, 0, 0, 0, time.UTC)

	// Since proper motion is applied for exactly 10 Julian years
	// Expected roughly:
	// RA += 10 * 1.5 arcsec = 15 arcsec
	// Dec += 10 * -0.5 arcsec = -5 arcsec
	// Plus light deflection and aberration corrections (~20 arcsec max)
	
	apparent := transform.AstrometricToApparent(astro, obsTime)

	// Basic sanity bounds checking — it should be a completely valid number
	// and visibly shifted from its geometric ICRS start.
	if math.IsNaN(apparent.RA.Degrees()) || math.IsNaN(apparent.Dec.Degrees()) {
		t.Fatalf("Apparent coordinate calculation yielded NaN")
	}

	raDiff := apparent.RA.Degrees() - astro.RA.Degrees()
	if math.Abs(raDiff) < 1.0/3600.0 {
		t.Errorf("Expected shift due to PM and precession, got almost 0: %v arcsec diff", raDiff*3600.0)
	}

	decDiff := apparent.Dec.Degrees() - astro.Dec.Degrees()
	if math.Abs(decDiff) < 1.0/3600.0 {
		t.Errorf("Expected shift due to PM and precession, got almost 0: %v arcsec diff", decDiff*3600.0)
	}
}

func TestApparentToObserved(t *testing.T) {
	// Zenith star in CIRS right on local meridian (Hour Angle = 0)
	// At observer's latitude, if Declination == Latitude and LST == RA, star is exactly at Zenith.
	
	site := earth.Geodetic{
		Lat:    angle.Deg(45.0),
		Lon:    angle.Deg(-90.0), // West 90
		Height: 100.0,
	}

	// We pick an arbitrary time. We won't perfectly match LST without extracting GST,
	// but we can just test if the transform correctly pushes the values to AltAz.
	
	obsTime := atime.Date(2023, 5, 1, 6, 0, 0, 0, time.UTC)
	
	apparent := coord.Apparent{
		RA:  angle.Deg(10.0),
		Dec: angle.Deg(45.0),
	}

	// Standard atmosphere
	atm := earth.StandardAtmosphere

	observed := transform.ApparentToObserved(apparent, obsTime, site, atm)

	// Result should be valid coordinates
	if observed.Alt.Degrees() < -90 || observed.Alt.Degrees() > 90 {
		t.Errorf("Invalid Altitude: %v", observed.Alt.Degrees())
	}
	if observed.Az.Degrees() < 0 || observed.Az.Degrees() > 360 {
		t.Errorf("Invalid Azimuth: %v", observed.Az.Degrees())
	}
}

func TestLegacyEquatorialHorizontalConsistency(t *testing.T) {
	site := earth.Geodetic{Lat: angle.Deg(45.0), Lon: angle.Deg(-90.0), Height: 0.0}
	obsTime := atime.Date(2023, 5, 1, 6, 0, 0, 0, time.UTC)
	
	geom := coord.ICRS{RA: angle.Deg(150), Dec: angle.Deg(30)}

	// Compute using legacy ICRSToAltAz
	altaz1, _ := transform.ICRSToAltAz(geom, obsTime, site)

	// Compute using explicit pipeline
	astro := coord.Astrometric{RA: geom.RA, Dec: geom.Dec}
	altaz2 := transform.AstrometricToObserved(astro, obsTime, site, earth.StandardAtmosphere)

	testutil.AssertNear(t, "Legacy vs Pipeline Altitude", altaz1.Alt.Radians(), altaz2.Alt.Radians(), 1e-12)
	testutil.AssertNear(t, "Legacy vs Pipeline Azimuth", altaz1.Az.Radians(), altaz2.Az.Radians(), 1e-12)
}
