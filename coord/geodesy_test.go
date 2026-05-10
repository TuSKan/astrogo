package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
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
	testutil.AssertNear(t, "Equator X", v1.X, wgs84.A, 1e-1)
	testutil.AssertNear(t, "Equator Y", v1.Y, 0, 1e-1)
	testutil.AssertNear(t, "Equator Z", v1.Z, 0, 1e-1)

	// North Pole
	g2, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(90), 0)
	v2 := g2.ToECEF(wgs84)
	b := wgs84.A * (1 - wgs84.F)

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
		g, _ := coord.NewGeodetic(c.lon, c.lat, c.h)
		v := g.ToECEF(wgs84)
		g2, err := coord.FromECEF(v, wgs84)

		label := testutil.CaseLabel(i, "RoundTrip")

		testutil.AssertNoError(t, err)
		testutil.AssertNear(t, label+" Lon", g2.Lon().Degrees(), g.Lon().Degrees(), 1e-9)
		testutil.AssertNear(t, label+" Lat", g2.Lat().Degrees(), g.Lat().Degrees(), 1e-9)
		testutil.AssertNear(t, label+" Height", g2.Height(), g.Height(), 1e-4)
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
