package transform_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/earth"
	"github.com/TuSKan/astrogo/frame"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/transform"
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
		icrs := coord.ICRS{RA: angle.Rad(c.ra), Dec: angle.Rad(c.dec)}
		gal := transform.ICRSToGalactic(icrs)
		back := transform.GalacticToICRS(gal)

		label := testutil.CaseLabel(i, "GalacticRoundTrip")
		testutil.AssertNear(t, label+" RA", back.RA.Radians(), icrs.RA.Radians(), 1e-12)
		testutil.AssertNear(t, label+" Dec", back.Dec.Radians(), icrs.Dec.Radians(), 1e-12)
	}
}

func TestEclipticRoundTrip(t *testing.T) {
	tm := time.FromJD(2451545.0, time.UTC) // J2000
	icrs := coord.ICRS{RA: angle.Deg(45), Dec: angle.Deg(30)}

	ecl := transform.ICRSToEcliptic(icrs, tm)
	back := transform.EclipticToICRS(ecl, tm)

	testutil.AssertNear(t, "Ecliptic RoundTrip RA", back.RA.Degrees(), icrs.RA.Degrees(), 1e-12)
	testutil.AssertNear(t, "Ecliptic RoundTrip Dec", back.Dec.Degrees(), icrs.Dec.Degrees(), 1e-12)
}

func TestAltAzRoundTrip(t *testing.T) {
	site, _ := earth.NewGeodetic(angle.Deg(10), angle.Deg(45), 500)
	tm := time.FromJD(2460000.5, time.UTC)
	icrs := coord.ICRS{RA: angle.Deg(100), Dec: angle.Deg(20)}

	aa, err := transform.ICRSToAltAz(icrs, tm, site)
	testutil.AssertNoError(t, err)

	back, err := transform.AltAzToICRS(aa, tm, site)
	testutil.AssertNoError(t, err)

	// Round-trip through refraction and Earth rotation should be very close.
	testutil.AssertNear(t, "AltAz RoundTrip RA", back.RA.Degrees(), icrs.RA.Degrees(), 1e-7)
	testutil.AssertNear(t, "AltAz RoundTrip Dec", back.Dec.Degrees(), icrs.Dec.Degrees(), 1e-7)
}

func TestGalacticPole(t *testing.T) {
	// North Galactic Pole (IAU 1958): RA = 192.85948, Dec = 27.12825
	ngp := coord.ICRS{
		RA:  angle.Deg(192.85948),
		Dec: angle.Deg(27.12825),
	}
	gal := transform.ICRSToGalactic(ngp)
	testutil.AssertNear(t, "NGP Latitude", gal.B.Degrees(), 90, 1e-5)
}

func TestAltAzEdgeCases(t *testing.T) {
	site, _ := earth.NewGeodetic(angle.Deg(0), angle.Deg(0), 0)
	tm := time.FromJD(2451545.0, time.UTC)

	// Zenith test
	icrsAtZenith := coord.ICRS{RA: angle.Deg(280.4606), Dec: angle.Deg(0)} // Simplified
	aa, _ := transform.ICRSToAltAz(icrsAtZenith, tm, site)
	// Just check it doesn't crash and returns reasonable values.
	if aa.Alt.Degrees() > 90 || aa.Alt.Degrees() < -90 {
		t.Errorf("Invalid Alt: %v", aa.Alt)
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
			icrs: coord.ICRS{RA: angle.Deg(192.85948), Dec: angle.Deg(27.12825)},
			b:    90,
		},
		{
			name: "South Galactic Pole",
			icrs: coord.ICRS{RA: angle.Deg(12.85948), Dec: angle.Deg(-27.12825)},
			b:    -90,
		},
		{
			name: "Galactic Center",
			icrs: coord.ICRS{RA: angle.Deg(266.405), Dec: angle.Deg(-28.936)},
			l:    0, b: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gal := transform.ICRSToGalactic(tt.icrs)
			if tt.b == 90 || tt.b == -90 {
				testutil.AssertNear(t, tt.name+" B", gal.B.Degrees(), tt.b, tol)
			} else {
				testutil.AssertNear(t, tt.name+" L", gal.L.Degrees(), tt.l, 0.1) // GC RA/Dec are approx
				testutil.AssertNear(t, tt.name+" B", gal.B.Degrees(), tt.b, 0.1)
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
			icrs: coord.ICRS{RA: angle.Deg(270), Dec: angle.Deg(66.5607083)},
			lat:  90,
		},
		{
			name: "South Ecliptic Pole",
			icrs: coord.ICRS{RA: angle.Deg(90), Dec: angle.Deg(-66.5607083)},
			lat:  -90,
		},
		{
			name: "Aries",
			icrs: coord.ICRS{RA: angle.Deg(0), Dec: angle.Deg(0)},
			lat:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ecl := transform.ICRSToEcliptic(tt.icrs, tm)
			testutil.AssertNear(t, tt.name+" Lat", ecl.Lat.Degrees(), tt.lat, tol)
		})
	}
}

func TestICRSToHourAngle(t *testing.T) {
	site, _ := earth.NewGeodetic(angle.Deg(0), angle.Deg(0), 0)
	tm := time.FromJD(2451545.0, time.UTC)
	icrs := coord.ICRS{RA: angle.Deg(0), Dec: angle.Deg(0)}
	ha, err := transform.ICRSToHourAngle(icrs, tm, site)
	testutil.AssertNoError(t, err)

	if math.IsNaN(ha.Radians()) {
		t.Errorf("Hour angle returning NaN")
	}
}

func TestTransformer(t *testing.T) {
	tr := transform.IdentityTransformer{}
	v := vector.V3(1, 2, 3)
	tm := time.FromJD(2451545.0, time.UTC)

	out, err := tr.Transform(frame.ICRS{}, frame.Galactic{}, v, tm)
	testutil.AssertNoError(t, err)

	if out.X != v.X || out.Y != v.Y || out.Z != v.Z {
		t.Errorf("IdentityTransformer changed vector, got %v", out)
	}
}
