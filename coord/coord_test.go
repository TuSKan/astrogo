package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestValidation(t *testing.T) {
	c := coord.NewICRS(angle.Deg(10), angle.Deg(45))
	testutil.AssertNoError(t, c.Validate())

	c2 := coord.NewICRS(angle.Deg(10), angle.Deg(95))
	testutil.AssertError(t, c2.Validate())

	c3 := coord.NewICRS(angle.Deg(10), angle.Rad(math.NaN()))
	testutil.AssertError(t, c3.Validate())
}

func TestToUnitVector_ICRS(t *testing.T) {
	c1 := coord.NewICRS(angle.Deg(0), angle.Deg(0))
	v1 := c1.ToUnitVector()
	testutil.AssertNear(t, "RA 0, Dec 0 -> X", v1.X, 1, 1e-15)
	testutil.AssertNear(t, "RA 0, Dec 0 -> Y", v1.Y, 0, 1e-15)
	testutil.AssertNear(t, "RA 0, Dec 0 -> Z", v1.Z, 0, 1e-15)

	c2 := coord.NewICRS(angle.Deg(0), angle.Deg(90))
	v2 := c2.ToUnitVector()
	testutil.AssertNear(t, "Dec 90 -> Z", v2.Z, 1, 1e-15)
}

func TestToUnitVector_AltAz(t *testing.T) {
	c1 := coord.NewAltAz(angle.Deg(0), angle.Deg(0))
	v1 := c1.ToUnitVector()
	testutil.AssertNear(t, "Alt 0, Az 0 -> X (North)", v1.X, 1, 1e-15)
	testutil.AssertNear(t, "Alt 0, Az 0 -> Y (East)", v1.Y, 0, 1e-15)
	testutil.AssertNear(t, "Alt 0, Az 0 -> Z (Up)", v1.Z, 0, 1e-15)

	c2 := coord.NewAltAz(angle.Deg(0), angle.Deg(90))
	v2 := c2.ToUnitVector()
	testutil.AssertNear(t, "Alt 0, Az 90 -> Y", v2.Y, 1, 1e-15)

	c3 := coord.NewAltAz(angle.Deg(90), angle.Deg(0))
	v3 := c3.ToUnitVector()
	testutil.AssertNear(t, "Zenith Z", v3.Z, 1, 1e-15)
}

func TestString(t *testing.T) {
	c := coord.NewICRS(angle.Hour(12.5), angle.Deg(-45.25))
	s := c.String()
	testutil.AssertEqual(t, "ICRS string", s, "ICRS RA=12h30m00.00s Dec=-45°15'00.00\"")
}

func TestAngleWrappingRequirement(t *testing.T) {
	c := coord.NewICRS(angle.Deg(370), angle.Deg(0))
	_ = c.String()
}

// ── FromUnitVector round-trip tests ──────────────────────────────────────────

func TestICRSRoundTrip(t *testing.T) {
	cases := []*coord.ICRS{
		coord.NewICRS(angle.Deg(0), angle.Deg(0)),
		coord.NewICRS(angle.Deg(90), angle.Deg(45)),
		coord.NewICRS(angle.Deg(180), angle.Deg(-30)),
		coord.NewICRS(angle.Deg(270), angle.Deg(0)),
		coord.NewICRS(angle.Deg(0), angle.Deg(89)),
		coord.NewICRS(angle.Deg(0), angle.Deg(-89)),
	}
	for i, c := range cases {
		v := c.ToUnitVector()
		var back coord.ICRS
		back.FromUnitVector(v)
		label := testutil.CaseLabel(i, "ICRS round-trip")
		testutil.AssertNear(t, label+" RA", back.RA().Degrees(), c.RA().Wrap360().Degrees(), 1e-10)
		testutil.AssertNear(t, label+" Dec", back.Dec().Degrees(), c.Dec().Degrees(), 1e-10)
	}
}

func TestGalacticRoundTrip_FromVec(t *testing.T) {
	cases := []*coord.Galactic{
		coord.NewGalactic(angle.Deg(0), angle.Deg(0)),
		coord.NewGalactic(angle.Deg(45), angle.Deg(20)),
		coord.NewGalactic(angle.Deg(180), angle.Deg(-45)),
	}
	for i, c := range cases {
		v := c.ToUnitVector()
		var back coord.Galactic
		back.FromUnitVector(v)
		label := testutil.CaseLabel(i, "Galactic round-trip")
		testutil.AssertNear(t, label+" L", back.L().Degrees(), c.L().Wrap360().Degrees(), 1e-10)
		testutil.AssertNear(t, label+" B", back.B().Degrees(), c.B().Degrees(), 1e-10)
	}
}

func TestEclipticRoundTrip_FromVec(t *testing.T) {
	cases := []*coord.Ecliptic{
		coord.NewEcliptic(angle.Deg(0), angle.Deg(0)),
		coord.NewEcliptic(angle.Deg(120), angle.Deg(-10)),
		coord.NewEcliptic(angle.Deg(300), angle.Deg(15)),
	}
	for i, c := range cases {
		v := c.ToUnitVector()
		var back coord.Ecliptic
		back.FromUnitVector(v)
		label := testutil.CaseLabel(i, "Ecliptic round-trip")
		testutil.AssertNear(t, label+" Lon", back.Lon().Degrees(), c.Lon().Wrap360().Degrees(), 1e-10)
		testutil.AssertNear(t, label+" Lat", back.Lat().Degrees(), c.Lat().Degrees(), 1e-10)
	}
}

// ── Equal tests ───────────────────────────────────────────────────────────────

func TestICRSEqual(t *testing.T) {
	a := coord.NewICRS(angle.Deg(45), angle.Deg(-15))
	b := coord.NewICRS(angle.Deg(45), angle.Deg(-15))
	c := coord.NewICRS(angle.Deg(45.001), angle.Deg(-15))

	if !a.Equal(b) {
		t.Error("identical ICRS should be equal")
	}
	if a.Equal(c) {
		t.Error("different ICRS should not be equal")
	}
}

func TestGalacticEqual(t *testing.T) {
	a := coord.NewGalactic(angle.Deg(120), angle.Deg(30))
	b := coord.NewGalactic(angle.Deg(120), angle.Deg(30))
	c := coord.NewGalactic(angle.Deg(121), angle.Deg(30))
	if !a.Equal(b) {
		t.Error("identical Galactic should be equal")
	}
	if a.Equal(c) {
		t.Error("different Galactic should not be equal")
	}
}

func TestEclipticEqual(t *testing.T) {
	a := coord.NewEcliptic(angle.Deg(60), angle.Deg(5))
	b := coord.NewEcliptic(angle.Deg(60), angle.Deg(5))
	if !a.Equal(b) {
		t.Error("identical Ecliptic should be equal")
	}
}

func TestAltAzEqual(t *testing.T) {
	a := coord.NewAltAz(angle.Deg(45), angle.Deg(180))
	b := coord.NewAltAz(angle.Deg(45), angle.Deg(180))
	if !a.Equal(b) {
		t.Error("identical AltAz should be equal")
	}
}
