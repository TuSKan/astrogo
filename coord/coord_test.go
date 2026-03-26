package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestValidation(t *testing.T) {
	c := coord.ICRS{RA: angle.Deg(10), Dec: angle.Deg(45)}
	testutil.AssertNoError(t, c.Validate())

	c2 := coord.ICRS{RA: angle.Deg(10), Dec: angle.Deg(95)}
	testutil.AssertError(t, c2.Validate())

	c3 := coord.ICRS{RA: angle.Deg(10), Dec: angle.Rad(math.NaN())}
	testutil.AssertError(t, c3.Validate())
}

func TestToUnitVector_ICRS(t *testing.T) {
	c1 := coord.ICRS{RA: angle.Deg(0), Dec: angle.Deg(0)}
	v1 := c1.ToUnitVector()
	testutil.AssertNear(t, "RA 0, Dec 0 -> X", v1.X, 1, 1e-15)
	testutil.AssertNear(t, "RA 0, Dec 0 -> Y", v1.Y, 0, 1e-15)
	testutil.AssertNear(t, "RA 0, Dec 0 -> Z", v1.Z, 0, 1e-15)

	c2 := coord.ICRS{RA: angle.Deg(0), Dec: angle.Deg(90)}
	v2 := c2.ToUnitVector()
	testutil.AssertNear(t, "Dec 90 -> Z", v2.Z, 1, 1e-15)
}

func TestToUnitVector_AltAz(t *testing.T) {
	c1 := coord.AltAz{Alt: angle.Deg(0), Az: angle.Deg(0)}
	v1 := c1.ToUnitVector()
	testutil.AssertNear(t, "Alt 0, Az 0 -> X (North)", v1.X, 1, 1e-15)
	testutil.AssertNear(t, "Alt 0, Az 0 -> Y (East)", v1.Y, 0, 1e-15)
	testutil.AssertNear(t, "Alt 0, Az 0 -> Z (Up)", v1.Z, 0, 1e-15)

	c2 := coord.AltAz{Alt: angle.Deg(0), Az: angle.Deg(90)}
	v2 := c2.ToUnitVector()
	testutil.AssertNear(t, "Alt 0, Az 90 -> Y", v2.Y, 1, 1e-15)

	c3 := coord.AltAz{Alt: angle.Deg(90), Az: angle.Deg(0)}
	v3 := c3.ToUnitVector()
	testutil.AssertNear(t, "Zenith Z", v3.Z, 1, 1e-15)
}

func TestString(t *testing.T) {
	c := coord.ICRS{RA: angle.Hour(12.5), Dec: angle.Deg(-45.25)}
	s := c.String()
	testutil.AssertEqual(t, "ICRS string", s, "ICRS RA=12h30m00.00s Dec=-45°15'00.00\"")
}

func TestAngleWrappingRequirement(t *testing.T) {
	c := coord.ICRS{RA: angle.Deg(370), Dec: angle.Deg(0)}
	_ = c.String()
}

// ── FromUnitVector round-trip tests ──────────────────────────────────────────

func TestICRSRoundTrip(t *testing.T) {
	cases := []coord.ICRS{
		{RA: angle.Deg(0), Dec: angle.Deg(0)},
		{RA: angle.Deg(90), Dec: angle.Deg(45)},
		{RA: angle.Deg(180), Dec: angle.Deg(-30)},
		{RA: angle.Deg(270), Dec: angle.Deg(0)},
		{RA: angle.Deg(0), Dec: angle.Deg(89)},
		{RA: angle.Deg(0), Dec: angle.Deg(-89)},
	}
	for i, c := range cases {
		v := c.ToUnitVector()
		back := coord.ICRSFromUnitVector(v)
		label := testutil.CaseLabel(i, "ICRS round-trip")
		testutil.AssertNear(t, label+" RA", back.RA.Degrees(), c.RA.Wrap360().Degrees(), 1e-10)
		testutil.AssertNear(t, label+" Dec", back.Dec.Degrees(), c.Dec.Degrees(), 1e-10)
	}
}

func TestGalacticRoundTrip_FromVec(t *testing.T) {
	cases := []coord.Galactic{
		{L: angle.Deg(0), B: angle.Deg(0)},
		{L: angle.Deg(45), B: angle.Deg(20)},
		{L: angle.Deg(180), B: angle.Deg(-45)},
	}
	for i, c := range cases {
		v := c.ToUnitVector()
		back := coord.GalacticFromUnitVector(v)
		label := testutil.CaseLabel(i, "Galactic round-trip")
		testutil.AssertNear(t, label+" L", back.L.Degrees(), c.L.Wrap360().Degrees(), 1e-10)
		testutil.AssertNear(t, label+" B", back.B.Degrees(), c.B.Degrees(), 1e-10)
	}
}

func TestEclipticRoundTrip_FromVec(t *testing.T) {
	cases := []coord.Ecliptic{
		{Lon: angle.Deg(0), Lat: angle.Deg(0)},
		{Lon: angle.Deg(120), Lat: angle.Deg(-10)},
		{Lon: angle.Deg(300), Lat: angle.Deg(15)},
	}
	for i, c := range cases {
		v := c.ToUnitVector()
		back := coord.EclipticFromUnitVector(v)
		label := testutil.CaseLabel(i, "Ecliptic round-trip")
		testutil.AssertNear(t, label+" Lon", back.Lon.Degrees(), c.Lon.Wrap360().Degrees(), 1e-10)
		testutil.AssertNear(t, label+" Lat", back.Lat.Degrees(), c.Lat.Degrees(), 1e-10)
	}
}

// ── Equal tests ───────────────────────────────────────────────────────────────

func TestICRSEqual(t *testing.T) {
	a := coord.ICRS{RA: angle.Deg(45), Dec: angle.Deg(-15)}
	b := coord.ICRS{RA: angle.Deg(45), Dec: angle.Deg(-15)}
	c := coord.ICRS{RA: angle.Deg(45.001), Dec: angle.Deg(-15)}

	if !a.Equal(b) {
		t.Error("identical ICRS should be equal")
	}
	if a.Equal(c) {
		t.Error("different ICRS should not be equal")
	}
}

func TestGalacticEqual(t *testing.T) {
	a := coord.Galactic{L: angle.Deg(120), B: angle.Deg(30)}
	b := coord.Galactic{L: angle.Deg(120), B: angle.Deg(30)}
	c := coord.Galactic{L: angle.Deg(121), B: angle.Deg(30)}
	if !a.Equal(b) {
		t.Error("identical Galactic should be equal")
	}
	if a.Equal(c) {
		t.Error("different Galactic should not be equal")
	}
}

func TestEclipticEqual(t *testing.T) {
	a := coord.Ecliptic{Lon: angle.Deg(60), Lat: angle.Deg(5)}
	b := coord.Ecliptic{Lon: angle.Deg(60), Lat: angle.Deg(5)}
	if !a.Equal(b) {
		t.Error("identical Ecliptic should be equal")
	}
}

func TestAltAzEqual(t *testing.T) {
	a := coord.AltAz{Alt: angle.Deg(45), Az: angle.Deg(180)}
	b := coord.AltAz{Alt: angle.Deg(45), Az: angle.Deg(180)}
	if !a.Equal(b) {
		t.Error("identical AltAz should be equal")
	}
}
