package coord_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

func TestNewGeodetic(t *testing.T) {
	// Valid coordinate
	g, err := coord.NewGeodetic(angle.Deg(10), angle.Deg(45), 500)
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "Lon degrees", g.Lon().Degrees(), 10, 1e-15)
	testutil.AssertNear(t, "Lat degrees", g.Lat().Degrees(), 45, 1e-15)

	// Invalid latitude
	_, err = coord.NewGeodetic(angle.Deg(0), angle.Deg(91), 0)
	testutil.AssertError(t, err)
	testutil.AssertErrorIs(t, err, coord.ErrLatitudeRange)

	// Non-finite
	_, err = coord.NewGeodetic(angle.Rad(math.NaN()), angle.Deg(0), 0)
	testutil.AssertError(t, err)
}

func TestECEF_EquatorAndPoles(t *testing.T) {
	wgs84 := coord.WGS84()

	// Equator, Lon 0
	g1, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(0), 0)
	v1 := g1.ToECEF(wgs84)
	testutil.AssertNear(t, "Equator X", v1.X, wgs84.A.Meters(), 1e-1)
	testutil.AssertNear(t, "Equator Y", v1.Y, 0, 1e-1)
	testutil.AssertNear(t, "Equator Z", v1.Z, 0, 1e-1)

	// North Pole
	g2, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(90), 0)
	v2 := g2.ToECEF(wgs84)
	b := wgs84.A.Meters() * (1 - wgs84.F)

	testutil.AssertNear(t, "Pole X", v2.X, 0, 1e-1)
	testutil.AssertNear(t, "Pole Y", v2.Y, 0, 1e-1)
	testutil.AssertNear(t, "Pole Z", v2.Z, b, 1e-1)
}

func TestECEFRoundTrip(t *testing.T) {
	wgs84 := coord.WGS84()
	cases := []struct {
		lon, lat angle.Angle
		h        float64
	}{
		{angle.Deg(0), angle.Deg(0), 0},
		{angle.Deg(45), angle.Deg(45), 1000},
		{angle.Deg(-120), angle.Deg(-30), -50},
		{angle.Deg(0), angle.Deg(89.9), 0},
		{angle.Deg(180), angle.Deg(0), 0},
	}

	for i, c := range cases {
		g, _ := coord.NewGeodetic(c.lon, c.lat, unit.Meters(c.h))
		v := g.ToECEF(wgs84)
		g2, err := coord.FromECEF(v, wgs84)

		label := testutil.CaseLabel(i, "RoundTrip")

		testutil.AssertNoError(t, err)
		testutil.AssertNear(t, label+" Lon", g2.Lon().Degrees(), g.Lon().Degrees(), 1e-9)
		testutil.AssertNear(t, label+" Lat", g2.Lat().Degrees(), g.Lat().Degrees(), 1e-9)
		testutil.AssertNear(t, label+" Height", g2.Height().Meters(), g.Height().Meters(), 1e-4)
	}
}

func TestECEF_ZeroVector(t *testing.T) {
	wgs84 := coord.WGS84()
	_, err := coord.FromECEF(vector.V3(0, 0, 0), wgs84)
	// FromECEF with Bowring should still return something, but we just check lack of panic.
	testutil.AssertNoError(t, err)
}

func TestGeodetic_Interface(t *testing.T) {
	g, _ := coord.NewGeodetic(angle.Deg(10), angle.Deg(20), 30)

	testutil.AssertEqual(t, "Name", g.Name(), "Geodetic")
	testutil.AssertNoError(t, g.Validate())

	s := g.String()
	testutil.AssertEqual(t, "String", s, "Lon=+10°00'00\", Lat=+20°00'00\", H=30.0m")

	// Equal
	g2, _ := coord.NewGeodetic(angle.Deg(10), angle.Deg(20), 30)
	g3, _ := coord.NewGeodetic(angle.Deg(10), angle.Deg(20), 40)

	if !g.Equal(g2) {
		t.Error("expected equal")
	}

	if g.Equal(g3) {
		t.Error("expected not equal")
	}

	// UnitVectors
	v := g.ToUnitVector()
	g4, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(0), 0)
	g4.FromUnitVector(v)
	testutil.AssertNear(t, "FromUnitVector Lon", g4.Lon().Degrees(), 10.0, 1e-10)
	testutil.AssertNear(t, "FromUnitVector Lat", g4.Lat().Degrees(), 20.0, 1e-10)
}

// TestFromECEFHoldsFromTheGroundToTheMoon round-trips geodetic → ECEF →
// geodetic across every latitude at heights from below the geoid to the
// Moon's distance. The forward direction is closed-form, so the round trip
// measures FromECEF alone.
//
// TestECEFRoundTrip above stays within a kilometer of the surface, where
// Bowring's (1976) single step — what FromECEF used before SOFA's Gc2gde —
// is exact. It degrades with altitude: its worst height error was 1.5 mm at
// the ISS, 31 cm at geostationary orbit and 42 cm at the Moon (#526), all
// near ±45° latitude. Measured here, the worst is 1.8e-7 m of height at the
// Moon's distance and 2.6e-11 rad of latitude at GPS orbit, both float64
// rounding on coordinates of up to 4e8 m.
func TestFromECEFHoldsFromTheGroundToTheMoon(t *testing.T) {
	t.Parallel()

	wgs84 := coord.WGS84()

	const (
		heightBound = 1e-6  // m: five times the worst measured, at the Moon
		latBound    = 1e-10 // rad: about 20 µas, four times the worst measured
	)

	for _, h := range []float64{-10e3, 0, 400e3, 20_200e3, 35_786e3, 384_400e3} {
		var worstH, worstLat float64

		for lat := -90.0; lat <= 90.0; lat += 0.25 {
			g, err := coord.NewGeodetic(angle.Deg(30), angle.Deg(lat), unit.Meters(h))
			testutil.AssertNoError(t, err)

			back, err := coord.FromECEF(g.ToECEF(wgs84), wgs84)
			testutil.AssertNoError(t, err)

			worstH = math.Max(worstH, math.Abs(back.Height().Meters()-h))
			worstLat = math.Max(worstLat, math.Abs(back.Lat().Radians()-g.Lat().Radians()))
		}

		t.Logf("height %9.0f km: worst height error %.3g m, latitude %.3g rad", h/1e3, worstH, worstLat)

		if worstH > heightBound || worstLat > latBound {
			t.Errorf("at %.0f km the round trip is off by %.3g m of height and %.3g rad of latitude, "+
				"want under %g m and %g rad", h/1e3, worstH, worstLat, heightBound, latBound)
		}
	}
}

// TestFromECEFOnThePolarAxis: no longitude is defined there, so it is 0, and
// the height is measured from the pole.
func TestFromECEFOnThePolarAxis(t *testing.T) {
	t.Parallel()

	wgs84 := coord.WGS84()
	b := wgs84.A.Meters() * (1 - wgs84.F)

	for _, z := range []float64{b + 1000, -(b + 1000)} {
		g, err := coord.FromECEF(vector.V3(0, 0, z), wgs84)
		testutil.AssertNoError(t, err)

		testutil.AssertNear(t, "polar latitude", math.Abs(g.Lat().Degrees()), 90, 1e-12)
		testutil.AssertNear(t, "polar longitude", g.Lon().Degrees(), 0, 0)
		testutil.AssertNear(t, "polar height", g.Height().Meters(), 1000, 1e-6)
	}
}

// TestFromECEFRefusesAnImpossibleEllipsoid: a radius that is not positive or
// a flattening outside [0, 1) is reported, not converted into NaNs.
func TestFromECEFRefusesAnImpossibleEllipsoid(t *testing.T) {
	t.Parallel()

	v := vector.V3(6378e3, 0, 0)

	for _, e := range []coord.Ellipsoid{
		{A: 0, F: coord.WGS84().F},
		{A: unit.Meters(-6378e3), F: coord.WGS84().F},
		{A: unit.Meters(6378e3), F: 1},
		{A: unit.Meters(6378e3), F: -0.1},
	} {
		if _, err := coord.FromECEF(v, e); !errors.Is(err, coord.ErrInvalidEllipsoid) {
			t.Errorf("FromECEF with a = %g m, f = %g: error %v, want ErrInvalidEllipsoid", e.A.Meters(), e.F, err)
		}
	}
}
