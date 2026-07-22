package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
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

func TestAngleWrappingRequirement(_ *testing.T) {
	c := coord.NewICRS(angle.Deg(370), angle.Deg(0))
	_ = c.String()
}

// ── FromUnitVector round-trip tests ──────────────────────────────────────────

func TestICRSRoundTrip(t *testing.T) {
	cases := []coord.ICRS{
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
	cases := []coord.Galactic{
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
	cases := []coord.Ecliptic{
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

func TestSetters(t *testing.T) {
	// ICRS
	ic := coord.NewICRS(angle.Deg(10), angle.Deg(20))
	ic.SetRA(angle.Deg(30))
	ic.SetDec(angle.Deg(40))
	ic.SetDist(5.0)
	testutil.AssertNear(t, "ICRS RA", ic.RA().Degrees(), 30.0, 1e-10)
	testutil.AssertNear(t, "ICRS Dec", ic.Dec().Degrees(), 40.0, 1e-10)
	testutil.AssertNear(t, "ICRS Dist", ic.Dist(), 5.0, 1e-10)

	// AltAz
	aa := coord.NewAltAz(angle.Deg(10), angle.Deg(20))
	aa.SetAlt(angle.Deg(30))
	aa.SetAz(angle.Deg(40))
	aa.SetDist(6.0)
	testutil.AssertNear(t, "AltAz Alt", aa.Alt().Degrees(), 30.0, 1e-10)
	testutil.AssertNear(t, "AltAz Az", aa.Az().Degrees(), 40.0, 1e-10)
	testutil.AssertNear(t, "AltAz Dist", aa.Dist(), 6.0, 1e-10)

	// Galactic
	gc := coord.NewGalactic(angle.Deg(10), angle.Deg(20))
	gc.SetL(angle.Deg(30))
	gc.SetB(angle.Deg(40))
	gc.SetDist(7.0)
	testutil.AssertNear(t, "Galactic L", gc.L().Degrees(), 30.0, 1e-10)
	testutil.AssertNear(t, "Galactic B", gc.B().Degrees(), 40.0, 1e-10)
	testutil.AssertNear(t, "Galactic Dist", gc.Dist(), 7.0, 1e-10)

	// Ecliptic
	ec := coord.NewEcliptic(angle.Deg(10), angle.Deg(20))
	ec.SetLon(angle.Deg(30))
	ec.SetLat(angle.Deg(40))
	ec.SetDist(8.0)
	testutil.AssertNear(t, "Ecliptic Lon", ec.Lon().Degrees(), 30.0, 1e-10)
	testutil.AssertNear(t, "Ecliptic Lat", ec.Lat().Degrees(), 40.0, 1e-10)
	testutil.AssertNear(t, "Ecliptic Dist", ec.Dist(), 8.0, 1e-10)

	// Astrometric
	am := coord.NewAstrometric(angle.Deg(10), angle.Deg(20))
	am.SetRA(angle.Deg(30))
	am.SetDec(angle.Deg(40))
	am.SetProperMotion(angle.Deg(1), angle.Deg(2))
	am.SetParallax(angle.Deg(0.5))
	am.SetRV(10.0)
	testutil.AssertNear(t, "Astrometric RA", am.RA().Degrees(), 30.0, 1e-10)
	testutil.AssertNear(t, "Astrometric Dec", am.Dec().Degrees(), 40.0, 1e-10)
	testutil.AssertNear(t, "Astrometric PmRA", am.PmRA().Degrees(), 1.0, 1e-10)
	testutil.AssertNear(t, "Astrometric PmDec", am.PmDec().Degrees(), 2.0, 1e-10)
	testutil.AssertNear(t, "Astrometric Parallax", am.Parallax().Degrees(), 0.5, 1e-10)
	testutil.AssertNear(t, "Astrometric RV", am.RV(), 10.0, 1e-10)

	// Apparent
	ap := coord.NewApparent(angle.Deg(10), angle.Deg(20))
	ap.SetRA(angle.Deg(30))
	ap.SetDec(angle.Deg(40))
	testutil.AssertNear(t, "Apparent RA", ap.RA().Degrees(), 30.0, 1e-10)
	testutil.AssertNear(t, "Apparent Dec", ap.Dec().Degrees(), 40.0, 1e-10)

	// ObserversLocation
	ol := coord.NewObserversLocation(angle.Deg(10), angle.Deg(20), 100)
	ol.SetLon(angle.Deg(30))
	ol.SetLat(angle.Deg(40))
	ol.SetHeight(200)
	testutil.AssertNear(t, "ObserversLocation Lon", ol.Lon().Degrees(), 30.0, 1e-10)
	testutil.AssertNear(t, "ObserversLocation Lat", ol.Lat().Degrees(), 40.0, 1e-10)
	testutil.AssertNear(t, "ObserversLocation Height", ol.Height(), 200.0, 1e-10)
}

func TestNames(t *testing.T) {
	testutil.AssertEqual(t, "Name", coord.NewAstrometric(angle.Deg(0), angle.Deg(0)).Name(), "Astrometric")
	testutil.AssertEqual(t, "Name", coord.NewApparent(angle.Deg(0), angle.Deg(0)).Name(), "Apparent")
	testutil.AssertEqual(t, "Name", coord.NewICRS(angle.Deg(0), angle.Deg(0)).Name(), "ICRS")
	testutil.AssertEqual(t, "Name", coord.NewAltAz(angle.Deg(0), angle.Deg(0)).Name(), "AltAz")
	testutil.AssertEqual(t, "Name", coord.NewGalactic(angle.Deg(0), angle.Deg(0)).Name(), "Galactic")
	testutil.AssertEqual(t, "Name", coord.NewEcliptic(angle.Deg(0), angle.Deg(0)).Name(), "Ecliptic")
	testutil.AssertEqual(t, "Name", coord.NewObserversLocation(angle.Deg(0), angle.Deg(0), 0).Name(), "ObserversLocation")
}

func TestValidations(t *testing.T) {
	err := coord.NewAstrometric(angle.Deg(0), angle.Deg(95)).Validate()
	testutil.AssertError(t, err)

	err = coord.NewApparent(angle.Deg(0), angle.Deg(95)).Validate()
	testutil.AssertError(t, err)

	err = coord.NewGalactic(angle.Deg(0), angle.Deg(95)).Validate()
	testutil.AssertError(t, err)

	err = coord.NewEcliptic(angle.Deg(0), angle.Deg(95)).Validate()
	testutil.AssertError(t, err)

	err = coord.NewAltAz(angle.Deg(95), angle.Deg(0)).Validate()
	testutil.AssertError(t, err)

	err = coord.NewObserversLocation(angle.Deg(0), angle.Deg(95), 0).Validate()
	testutil.AssertError(t, err)
}

func TestMoreEqual(t *testing.T) {
	astA := coord.NewAstrometric(angle.Deg(1), angle.Deg(2))
	astB := coord.NewAstrometric(angle.Deg(1), angle.Deg(2))
	astC := coord.NewAstrometric(angle.Deg(2), angle.Deg(2))

	if !astA.Equal(astB) {
		t.Error("identical Astrometric should be equal")
	}

	if astA.Equal(astC) {
		t.Error("different Astrometric should not be equal")
	}

	appA := coord.NewApparent(angle.Deg(1), angle.Deg(2))

	appB := coord.NewApparent(angle.Deg(1), angle.Deg(2))
	if !appA.Equal(appB) {
		t.Error("identical Apparent should be equal")
	}

	obsA := coord.NewObserversLocation(angle.Deg(1), angle.Deg(2), 10)
	obsB := coord.NewObserversLocation(angle.Deg(1), angle.Deg(2), 10)
	obsC := coord.NewObserversLocation(angle.Deg(1), angle.Deg(2), 20)

	if !obsA.Equal(obsB) {
		t.Error("identical ObserversLocation should be equal")
	}

	if obsA.Equal(obsC) {
		t.Error("different ObserversLocation should not be equal")
	}
}

func TestStrings(t *testing.T) {
	ast := coord.NewAstrometric(angle.Deg(15), angle.Deg(30))
	testutil.AssertEqual(t, "Astrometric string", ast.String(), "Astrometric RA=01h00m00.00s Dec=+30°00'00.00\"")

	app := coord.NewApparent(angle.Deg(15), angle.Deg(30))
	testutil.AssertEqual(t, "Apparent string", app.String(), "Apparent RA=01h00m00.00s Dec=+30°00'00.00\"")

	aa := coord.NewAltAz(angle.Deg(30), angle.Deg(15))
	testutil.AssertEqual(t, "AltAz string", aa.String(), "AltAz Alt=+30°00'00.00\" Az=+15°00'00.00\"")

	gal := coord.NewGalactic(angle.Deg(15), angle.Deg(30))
	testutil.AssertEqual(t, "Galactic string", gal.String(), "Galactic L=+15°00'00.00\" B=+30°00'00.00\"")

	ecl := coord.NewEcliptic(angle.Deg(15), angle.Deg(30))
	testutil.AssertEqual(t, "Ecliptic string", ecl.String(), "Ecliptic Lon=+15°00'00.00\" Lat=+30°00'00.00\"")

	obs := coord.NewObserversLocation(angle.Deg(15), angle.Deg(30), 100)
	testutil.AssertEqual(t, "ObserversLocation string", obs.String(), "ObserversLocation Lon=+15°00'00.00\" Lat=+30°00'00.00\" Height=100.000000")
}

func TestSeparationAndPositionAngle(t *testing.T) {
	ic1 := coord.NewICRS(angle.Deg(0), angle.Deg(0))
	ic2 := coord.NewICRS(angle.Deg(90), angle.Deg(0))
	sep := coord.Separation(ic1, ic2)
	testutil.AssertNear(t, "Separation", sep.Degrees(), 90.0, 1e-10)

	pa := coord.PositionAngle(ic1, ic2)
	testutil.AssertNear(t, "PositionAngle", pa.Degrees(), 90.0, 1e-10)
}

func TestPropagateEpoch_NoKinematicsIsNoOp(t *testing.T) {
	c := coord.NewICRS(angle.Deg(10), angle.Deg(20))

	later := time.J2000.Add((50 * 365.25 * 24) * time.Hour)

	out, err := coord.PropagateEpoch(c, time.J2000, later)
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "RA unchanged with no kinematics", out.RA().Degrees(), c.RA().Degrees(), 1e-12)
	testutil.AssertNear(t, "Dec unchanged with no kinematics", out.Dec().Degrees(), c.Dec().Degrees(), 1e-12)
}

func TestPropagateEpoch_SameEpochIsNoOp(t *testing.T) {
	c := coord.NewICRSWithKinematics(angle.Deg(10), angle.Deg(0), angle.Arcsec(1), angle.Arcsec(0), angle.Arcsec(0.1), 0)

	out, err := coord.PropagateEpoch(c, time.J2000, time.J2000)
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "RA unchanged at same epoch", out.RA().Degrees(), c.RA().Degrees(), 1e-15)
}

func TestPropagateEpoch_AppliesProperMotion(t *testing.T) {
	// dec=0 so cos(dec)=1: coordinate-angle pmRA of 1 arcsec/yr over 10
	// years should shift RA by ~10 arcsec, to within a small tolerance for
	// the relativistic/parallax correction Pmsafe also applies.
	c := coord.NewICRSWithKinematics(angle.Deg(10), angle.Deg(0), angle.Arcsec(1), angle.Arcsec(0), angle.Arcsec(0.1), 0)

	later := time.J2000.Add((10 * 365.25 * 24) * time.Hour)

	out, err := coord.PropagateEpoch(c, time.J2000, later)
	testutil.AssertNoError(t, err)

	shiftArcsec := (out.RA().Degrees() - c.RA().Degrees()) * 3600
	testutil.AssertNear(t, "RA shift over 10yr at 1 arcsec/yr PM", shiftArcsec, 10.0, 0.1)
}

func TestPropagateEpoch_ZeroEpochDefaultsToJ2000(t *testing.T) {
	c := coord.NewICRSWithKinematics(angle.Deg(10), angle.Deg(0), angle.Arcsec(1), angle.Arcsec(0), angle.Arcsec(0.1), 0)

	later := time.J2000.Add((10 * 365.25 * 24) * time.Hour)

	explicit, err := coord.PropagateEpoch(c, time.J2000, later)
	testutil.AssertNoError(t, err)

	implicit, err := coord.PropagateEpoch(c, time.Time{}, later)
	testutil.AssertNoError(t, err)

	testutil.AssertNear(t, "zero fromEpoch matches explicit J2000", implicit.RA().Degrees(), explicit.RA().Degrees(), 1e-15)
}

func TestMoreRoundTrips(t *testing.T) {
	ast := coord.NewAstrometric(angle.Deg(45), angle.Deg(45))
	v := ast.ToUnitVector()
	ast2 := coord.NewAstrometric(angle.Deg(0), angle.Deg(0))
	ast2.FromUnitVector(v)
	testutil.AssertNear(t, "astronomical roundtrip", ast2.RA().Degrees(), 45.0, 1e-10)

	app := coord.NewApparent(angle.Deg(45), angle.Deg(45))
	v = app.ToUnitVector()
	app2 := coord.NewApparent(angle.Deg(0), angle.Deg(0))
	app2.FromUnitVector(v)
	testutil.AssertNear(t, "apparent roundtrip", app2.RA().Degrees(), 45.0, 1e-10)

	obs := coord.NewObserversLocation(angle.Deg(45), angle.Deg(45), 100)
	v = obs.ToUnitVector()
	obs2 := coord.NewObserversLocation(angle.Deg(0), angle.Deg(0), 0)
	obs2.FromUnitVector(v)
	testutil.AssertNear(t, "observers roundtrip", obs2.Lon().Degrees(), 45.0, 1e-10)

	aa := coord.NewAltAz(angle.Deg(45), angle.Deg(45))
	v = aa.ToUnitVector()
	aa2 := coord.NewAltAz(angle.Deg(0), angle.Deg(0))
	aa2.FromUnitVector(v)
	testutil.AssertNear(t, "altaz roundtrip", aa2.Az().Degrees(), 45.0, 1e-10)
}
