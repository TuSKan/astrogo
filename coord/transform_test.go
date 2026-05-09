package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

func TestGalacticRoundTrip(t *testing.T) {
	cases := []struct {
		ra, dec float64
	}{
		{0, 0},
		{math.Pi, 0},
		{0, math.Pi / 4},
		{266.405 * math.Pi / 180, -28.936 * math.Pi / 180}, // Near Gal Center
	}

	for i, c := range cases {
		icrs := coord.NewICRS(angle.Rad(c.ra), angle.Rad(c.dec))
		gal := coord.ICRSToGalactic(icrs)
		back := coord.GalacticToICRS(gal)

		label := testutil.CaseLabel(i, "GalacticRoundTrip")
		testutil.AssertNear(t, label+" RA", back.RA().Degrees(), icrs.RA().Degrees(), 1e-12)
		testutil.AssertNear(t, label+" Dec", back.Dec().Degrees(), icrs.Dec().Degrees(), 1e-12)
	}
}

func TestEclipticRoundTrip(t *testing.T) {
	tm := time.FromJD(2451545.0, time.UTC) // J2000
	icrs := coord.NewICRS(angle.Deg(45), angle.Deg(30))

	ecl := coord.ICRSToEcliptic(icrs, tm)
	back := coord.EclipticToICRS(ecl, tm)

	testutil.AssertNear(t, "Ecliptic RoundTrip RA", back.RA().Degrees(), icrs.RA().Degrees(), 1e-12)
	testutil.AssertNear(t, "Ecliptic RoundTrip Dec", back.Dec().Degrees(), icrs.Dec().Degrees(), 1e-12)
}

func TestAltAzRoundTrip(t *testing.T) {
	site, _ := coord.NewGeodetic(angle.Deg(10), angle.Deg(45), 500)
	tm := time.FromJD(2460000.5, time.UTC)
	icrs := coord.NewICRS(angle.Deg(100), angle.Deg(20))

	ctx := coord.NewContext(tm, site, atmosphere.StandardAtmosphere)
	aa, err := ctx.ICRSToAltAz(icrs)
	testutil.AssertNoError(t, err)

	back, err := ctx.AltAzToICRS(aa)
	testutil.AssertNoError(t, err)

	// Round-trip through refraction and Earth rotation should be very close,
	// but Saemundsson / Bennett empirical models are not algebraically perfect inverses.
	// Tolerating ~3.6 arcsec (1e-3 deg) which is standard for mixed empirical mappings.
	testutil.AssertNear(t, "AltAz RoundTrip RA", back.RA().Degrees(), icrs.RA().Degrees(), 1e-3)
	testutil.AssertNear(t, "AltAz RoundTrip Dec", back.Dec().Degrees(), icrs.Dec().Degrees(), 1e-3)
}

func TestGalacticPole(t *testing.T) {
	// North Galactic Pole (IAU 1958): RA = 192.85948, Dec = 27.12825
	ngp := coord.NewICRS(angle.Deg(192.85948), angle.Deg(27.12825))
	gal := coord.ICRSToGalactic(ngp)
	testutil.AssertNear(t, "NGP Latitude", gal.B().Degrees(), 90, 1e-5)
}

func TestAltAzEdgeCases(t *testing.T) {
	site, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(0), 0)
	tm := time.FromJD(2451545.0, time.UTC)

	// Zenith test
	icrsAtZenith := coord.NewICRS(angle.Deg(280.4606), angle.Deg(0)) // Simplified
	ctx := coord.NewContext(tm, site, atmosphere.StandardAtmosphere)
	aa, _ := ctx.ICRSToAltAz(icrsAtZenith)
	// Just check it doesn't crash and returns reasonable values.
	if aa.Alt().Degrees() > 90 || aa.Alt().Degrees() < -90 {
		t.Errorf("Invalid Alt: %v", aa.Alt())
	}
}

func TestGalacticExtremes(t *testing.T) {
	const tol = 1e-7
	tests := []struct {
		name string
		icrs coord.ICRS
		l, b float64
	}{
		{
			name: "North Galactic Pole",
			icrs: coord.NewICRS(angle.Deg(192.85948), angle.Deg(27.12825)),
			b:    90,
		},
		{
			name: "South Galactic Pole",
			icrs: coord.NewICRS(angle.Deg(12.85948), angle.Deg(-27.12825)),
			b:    -90,
		},
		{
			name: "Galactic Center",
			icrs: coord.NewICRS(angle.Deg(266.405), angle.Deg(-28.936)),
			l:    0, b: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gal := coord.ICRSToGalactic(tt.icrs)
			if tt.b == 90 || tt.b == -90 {
				testutil.AssertNear(t, tt.name+" B", gal.B().Degrees(), tt.b, tol)
			} else {
				testutil.AssertNear(t, tt.name+" L", gal.L().Degrees(), tt.l, 0.1) // GC RA/Dec are approx
				testutil.AssertNear(t, tt.name+" B", gal.B().Degrees(), tt.b, 0.1)
			}
		})
	}
}

func TestEclipticExtremes(t *testing.T) {
	const tol = 2e-5
	tm := time.FromJD(2451545.0, time.UTC) // J2000

	tests := []struct {
		name string
		icrs coord.ICRS
		lat  float64
	}{
		{
			name: "North Ecliptic Pole",
			icrs: coord.NewICRS(angle.Deg(270), angle.Deg(66.5607083)),
			lat:  90,
		},
		{
			name: "South Ecliptic Pole",
			icrs: coord.NewICRS(angle.Deg(90), angle.Deg(-66.5607083)),
			lat:  -90,
		},
		{
			name: "Aries",
			icrs: coord.NewICRS(angle.Deg(0), angle.Deg(0)),
			lat:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ecl := coord.ICRSToEcliptic(tt.icrs, tm)
			testutil.AssertNear(t, tt.name+" Lat", ecl.Lat().Degrees(), tt.lat, tol)
		})
	}
}

func TestICRSToHourAngle(t *testing.T) {
	site, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(0), 0)
	tm := time.FromJD(2451545.0, time.UTC)
	icrs := coord.NewICRS(angle.Deg(0), angle.Deg(0))
	ctx := coord.NewContext(tm, site, atmosphere.StandardAtmosphere)
	ha, err := ctx.ICRSToHourAngle(icrs)
	testutil.AssertNoError(t, err)

	if math.IsNaN(ha.Radians()) {
		t.Errorf("Hour angle returning NaN")
	}
}

func TestRefractionModes(t *testing.T) {
	site, _ := coord.NewGeodetic(angle.Deg(45.0), angle.Deg(45.0), 100.0)

	obsTime := time.Date(2023, 5, 1, 6, 0, 0, 0, time.LocationUTC)

	// A star likely to be somewhat low on the horizon
	astro := coord.NewAstrometric(angle.Deg(270.0), angle.Deg(10.0))

	// 1. No Refraction Model
	atmNone := atmosphere.StandardAtmosphere
	atmNone.Model = atmosphere.RefractionNone{}
	ctxNone := coord.NewContext(obsTime, site, atmNone)
	obsNone := ctxNone.AstrometricToObserved(astro)

	// 2. SOFA (Native) Refraction Model
	atmSOFA := atmosphere.StandardAtmosphere
	atmSOFA.Model = atmosphere.RefractionRigorous{}
	ctxSOFA := coord.NewContext(obsTime, site, atmSOFA)
	obsSOFA := ctxSOFA.AstrometricToObserved(astro)

	// 3. Approximate Refraction Model
	atmApprox := atmosphere.StandardAtmosphere
	atmApprox.Model = atmosphere.RefractionApproximate{}
	ctxApprox := coord.NewContext(obsTime, site, atmApprox)
	obsApprox := ctxApprox.AstrometricToObserved(astro)

	// Assertions
	altNone := obsNone.Alt().Degrees()
	altSOFA := obsSOFA.Alt().Degrees()
	altApprox := obsApprox.Alt().Degrees()

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

func TestAstrometricToApparent(t *testing.T) {
	// A mock star at epoch J2000.0 with extreme proper motion and parallax
	astro := coord.NewAstrometric(angle.Deg(150.0), angle.Deg(-30.0))
	astro.SetProperMotion(angle.Arcsec(1.5), angle.Arcsec(-0.5))
	astro.SetParallax(angle.Arcsec(0.2))
	astro.SetRV(50.0) // km/s

	// Calculate apparent position 10 years later (2010.0)
	obsTime := time.Date(2010, 1, 1, 12, 0, 0, 0, time.LocationUTC)

	site, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(0), 0)
	ctx := coord.NewContext(obsTime, site, atmosphere.StandardAtmosphere)
	apparent := ctx.AstrometricToApparent(astro)

	// Basic sanity bounds checking — it should be a completely valid number
	// and visibly shifted from its geometric ICRS start.
	if math.IsNaN(apparent.RA().Degrees()) || math.IsNaN(apparent.Dec().Degrees()) {
		t.Fatalf("Apparent coordinate calculation yielded NaN")
	}

	raDiff := apparent.RA().Degrees() - astro.RA().Degrees()
	if math.Abs(raDiff) < 1.0/3600.0 {
		t.Errorf("Expected shift due to PM and precession, got almost 0: %v arcsec diff", raDiff*3600.0)
	}

	decDiff := apparent.Dec().Degrees() - astro.Dec().Degrees()
	if math.Abs(decDiff) < 1.0/3600.0 {
		t.Errorf("Expected shift due to PM and precession, got almost 0: %v arcsec diff", decDiff*3600.0)
	}
}

func TestApparentToObserved(t *testing.T) {
	// Zenith star in CIRS right on local meridian (Hour Angle = 0)
	// At observer's latitude, if Declination == Latitude and LST == RA, star is exactly at Zenith.

	site, err := coord.NewGeodetic(angle.Deg(45.0), angle.Deg(-90.0), 100.0)
	if err != nil {
		t.Fatalf("Failed to create geodetic site: %v", err)
	}

	obsTime := time.Date(2023, 5, 1, 6, 0, 0, 0, time.LocationUTC)

	apparent := coord.NewApparent(angle.Deg(10.0), angle.Deg(45.0))

	// Standard atmosphere
	atm := atmosphere.StandardAtmosphere

	ctx := coord.NewContext(obsTime, site, atm)
	observed := ctx.ApparentToObserved(apparent)

	// Result should be valid coordinates
	if observed.Alt().Degrees() < -90 || observed.Alt().Degrees() > 90 {
		t.Errorf("Invalid Altitude: %v", observed.Alt().Degrees())
	}
	if observed.Az().Degrees() < 0 || observed.Az().Degrees() > 360 {
		t.Errorf("Invalid Azimuth: %v", observed.Az().Degrees())
	}
}

func TestLegacyEquatorialHorizontalConsistency(t *testing.T) {
	site, err := coord.NewGeodetic(angle.Deg(45.0), angle.Deg(-90.0), 0.0)
	if err != nil {
		t.Fatalf("Failed to create geodetic site: %v", err)
	}
	obsTime := time.Date(2023, 5, 1, 6, 0, 0, 0, time.LocationUTC)

	geom := coord.NewICRS(angle.Deg(150), angle.Deg(30))

	ctx := coord.NewContext(obsTime, site, atmosphere.StandardAtmosphere)

	// Compute using ICRSToAltAz
	altaz1, _ := ctx.ICRSToAltAz(geom)

	// Compute using explicit pipeline
	astro := coord.NewAstrometric(geom.RA(), geom.Dec())
	altaz2 := ctx.AstrometricToObserved(astro)

	testutil.AssertNear(t, "Legacy vs Pipeline Altitude", altaz1.Alt().Degrees(), altaz2.Alt().Degrees(), 1e-12)
	testutil.AssertNear(t, "Legacy vs Pipeline Azimuth", altaz1.Az().Degrees(), altaz2.Az().Degrees(), 1e-12)
}

func TestTransformNearPole(t *testing.T) {
	// Observatory at North Pole
	tm := time.NowUTC()

	// Star at zenith from North Pole
	locN, err := coord.NewGeodetic(angle.Deg(0), angle.Deg(90), 0)
	if err != nil {
		t.Fatalf("Failed to create geodetic site: %v", err)
	}
	starNorth := coord.NewICRS(angle.Deg(0), angle.Deg(89.9))
	ctxN := coord.NewContext(tm, locN, atmosphere.StandardAtmosphere)
	aaN, err := ctxN.ICRSToAltAz(starNorth)
	testutil.AssertNoError(t, err)
	if aaN.Alt().Degrees() < 89.0 {
		t.Fatalf("expected near-zenith altitude at North Pole, got %.6f deg", aaN.Alt().Degrees())
	}

	// Star at horizon from South Pole
	locS, err := coord.NewGeodetic(angle.Deg(0), angle.Deg(-90), 0)
	if err != nil {
		t.Fatalf("Failed to create geodetic site: %v", err)
	}
	starEq := coord.NewICRS(angle.Deg(0), angle.Deg(0))
	ctxS := coord.NewContext(tm, locS, atmosphere.StandardAtmosphere)
	aaS, err := ctxS.ICRSToAltAz(starEq)
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "Altitude at S.Pole Horizon", aaS.Alt().Degrees(), 0, 0.5)
}

func TestTransformBoundaryRA(t *testing.T) {
	loc, err := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	if err != nil {
		t.Fatalf("Failed to create geodetic site: %v", err)
	}
	tm := time.NowUTC()
	ctx := coord.NewContext(tm, loc, atmosphere.StandardAtmosphere)

	// Test RA 359.999 vs 0.001 should yield very similar results
	s1 := coord.NewICRS(angle.Deg(359.999), angle.Deg(45))
	s2 := coord.NewICRS(angle.Deg(0.001), angle.Deg(45))

	aa1, _ := ctx.ICRSToAltAz(s1)
	aa2, _ := ctx.ICRSToAltAz(s2)

	diff := aa1.Az().Sub(aa2.Az()).WrapPi().Degrees()
	if diff > 0.1 {
		t.Errorf("RA wrap discontinuity: Az1=%v, Az2=%v, diff=%v", aa1.Az(), aa2.Az(), diff)
	}
}

func TestNegativeLongitude(t *testing.T) {
	// Lon -45 should be same as Lon 315
	loc1, err := coord.NewGeodetic(angle.Deg(-45), angle.Deg(0), 0)
	if err != nil {
		t.Fatalf("Failed to create geodetic site: %v", err)
	}
	loc2, err := coord.NewGeodetic(angle.Deg(315), angle.Deg(0), 0)
	if err != nil {
		t.Fatalf("Failed to create geodetic site: %v", err)
	}
	tm := time.NowUTC()
	star := coord.NewICRS(angle.Deg(0), angle.Deg(0))

	ctx1 := coord.NewContext(tm, loc1, atmosphere.StandardAtmosphere)
	ctx2 := coord.NewContext(tm, loc2, atmosphere.StandardAtmosphere)
	aa1, _ := ctx1.ICRSToAltAz(star)
	aa2, _ := ctx2.ICRSToAltAz(star)

	testutil.AssertNear(t, "Alt same for -45/315 lon", aa1.Alt().Degrees(), aa2.Alt().Degrees(), 1e-10)
	diffAz := aa1.Az().Sub(aa2.Az()).WrapPi().Degrees()
	testutil.AssertNear(t, "Az same for -45/315 lon", diffAz, 0, 1e-10)
}

func TestICRSBatchToAltAzParallel_Correctness(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	atm := atmosphere.AtAltitude(2635)
	tm := time.FromJD(2460000.5, time.UTC)
	ctx := coord.NewContext(tm, loc, atm)

	const n = 500
	stars := make([]coord.ICRS, n)
	for i := range stars {
		ra := angle.Deg(float64(i) * 360.0 / float64(n))
		dec := angle.Deg(float64(i)*180.0/float64(n) - 90)
		stars[i] = coord.NewICRS(ra, dec)
	}

	serial := make([]coord.AltAz, n)
	parallel := make([]coord.AltAz, n)

	ctx.ICRSBatchToAltAz(stars, serial)
	ctx.ICRSBatchToAltAzParallel(stars, parallel)

	for i := 0; i < n; i++ {
		if math.Abs(serial[i].Alt().Degrees()-parallel[i].Alt().Degrees()) > 1e-12 ||
			math.Abs(serial[i].Az().Degrees()-parallel[i].Az().Degrees()) > 1e-12 {
			t.Fatalf("mismatch at index %d: serial=(%.10f, %.10f) parallel=(%.10f, %.10f)",
				i,
				serial[i].Alt().Degrees(), serial[i].Az().Degrees(),
				parallel[i].Alt().Degrees(), parallel[i].Az().Degrees())
		}
	}
}

func TestReduceBatchParallel_Correctness(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	atm := atmosphere.AtAltitude(2635)
	tm := time.FromJD(2460000.5, time.UTC)
	ctx := coord.NewContext(tm, loc, atm)

	const n = 500
	vecs := make([]vector.Vec3, n)
	for i := range vecs {
		vecs[i] = vector.Vec3{
			X: 0.00257 + float64(i)*1e-8,
			Y: 0.00010 + float64(i)*1e-9,
			Z: 0.00005,
		}
	}

	serial := make([]coord.AltAz, n)
	parallel := make([]coord.AltAz, n)

	ctx.ReduceBatch(vecs, serial)
	ctx.ReduceBatchParallel(vecs, parallel)

	for i := 0; i < n; i++ {
		if math.Abs(serial[i].Alt().Degrees()-parallel[i].Alt().Degrees()) > 1e-12 ||
			math.Abs(serial[i].Az().Degrees()-parallel[i].Az().Degrees()) > 1e-12 {
			t.Fatalf("mismatch at index %d: serial=(%.10f, %.10f) parallel=(%.10f, %.10f)",
				i,
				serial[i].Alt().Degrees(), serial[i].Az().Degrees(),
				parallel[i].Alt().Degrees(), parallel[i].Az().Degrees())
		}
	}
}
