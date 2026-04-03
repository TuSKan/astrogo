package earth_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/earth"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/vector"
)

func TestNewGeodetic(t *testing.T) {
	// Valid coordinate
	g, err := earth.NewGeodetic(angle.Deg(10), angle.Deg(45), 500)
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "Lon degrees", g.Lon.Degrees(), 10, 1e-15)
	testutil.AssertNear(t, "Lat degrees", g.Lat.Degrees(), 45, 1e-15)

	// Invalid latitude
	_, err = earth.NewGeodetic(angle.Deg(0), angle.Deg(91), 0)
	testutil.AssertError(t, err)
	testutil.AssertErrorContains(t, err, "latitude must be between -90 and 90 degrees")

	// Non-finite
	_, err = earth.NewGeodetic(angle.Rad(math.NaN()), angle.Deg(0), 0)
	testutil.AssertError(t, err)
}

func TestECEF_EquatorAndPoles(t *testing.T) {
	wgs84 := earth.WGS84()

	// Equator, Lon 0
	g1, _ := earth.NewGeodetic(angle.Deg(0), angle.Deg(0), 0)
	v1 := g1.ToECEF(wgs84)
	testutil.AssertNear(t, "Equator X", v1.X, wgs84.A, 1e-1)
	testutil.AssertNear(t, "Equator Y", v1.Y, 0, 1e-1)
	testutil.AssertNear(t, "Equator Z", v1.Z, 0, 1e-1)

	// North Pole
	g2, _ := earth.NewGeodetic(angle.Deg(0), angle.Deg(90), 0)
	v2 := g2.ToECEF(wgs84)
	b := wgs84.A * (1 - wgs84.F)
	testutil.AssertNear(t, "Pole X", v2.X, 0, 1e-1)
	testutil.AssertNear(t, "Pole Y", v2.Y, 0, 1e-1)
	testutil.AssertNear(t, "Pole Z", v2.Z, b, 1e-1)
}

func TestECEFRoundTrip(t *testing.T) {
	wgs84 := earth.WGS84()
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
		g, _ := earth.NewGeodetic(c.lon, c.lat, c.h)
		v := g.ToECEF(wgs84)
		g2, err := earth.FromECEF(v, wgs84)

		label := testutil.CaseLabel(i, "RoundTrip")
		testutil.AssertNoError(t, err)
		testutil.AssertNear(t, label+" Lon", g2.Lon.Degrees(), g.Lon.Degrees(), 1e-9)
		testutil.AssertNear(t, label+" Lat", g2.Lat.Degrees(), g.Lat.Degrees(), 1e-9)
		testutil.AssertNear(t, label+" Height", g2.Height, g.Height, 1e-4)
	}
}

func TestECEF_ZeroVector(t *testing.T) {
	wgs84 := earth.WGS84()
	_, err := earth.FromECEF(vector.V3(0, 0, 0), wgs84)
	// FromECEF with Bowring should still return something, but we just check lack of panic.
	testutil.AssertNoError(t, err)
}
