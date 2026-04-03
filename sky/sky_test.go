package sky_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/earth"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/observatory"
	"github.com/TuSKan/astrogo/sky"
	"github.com/TuSKan/astrogo/time"
)

func TestSeparation(t *testing.T) {
	a := coord.ICRS{RA: angle.Deg(0), Dec: angle.Deg(0)}
	b := coord.ICRS{RA: angle.Deg(0), Dec: angle.Deg(10)}

	sep := sky.Separation(a, b)
	testutil.AssertNear(t, "Separation(0, 10)", sep.Degrees(), 10, 1e-12)

	// Antipode
	c := coord.ICRS{RA: angle.Deg(180), Dec: angle.Deg(0)}
	sep2 := sky.Separation(a, c)
	testutil.AssertNear(t, "Separation(0, 180)", sep2.Degrees(), 180, 1e-12)
}

func TestPositionAngle(t *testing.T) {
	from := coord.ICRS{RA: angle.Deg(0), Dec: angle.Deg(0)}
	to := coord.ICRS{RA: angle.Deg(0), Dec: angle.Deg(1)}

	pa := sky.PositionAngle(from, to)
	testutil.AssertNear(t, "PA due North", pa.Degrees(), 0, 1e-12)

	toE := coord.ICRS{RA: angle.Deg(0.1), Dec: angle.Deg(0)}
	paE := sky.PositionAngle(from, toE)
	testutil.AssertNear(t, "PA due East", paE.Degrees(), 90, 1e-10)
}

func TestSeparationEdge(t *testing.T) {
	// Near antipode: 179.999 degrees
	a := coord.ICRS{RA: angle.Deg(0), Dec: angle.Deg(0)}
	b := coord.ICRS{RA: angle.Deg(179.999), Dec: angle.Deg(0)}
	sep := sky.Separation(a, b)
	testutil.AssertNear(t, "Separation near antipode", sep.Degrees(), 179.999, 1e-10)

	// Antipode exactly
	c := coord.ICRS{RA: angle.Deg(180), Dec: angle.Deg(0)}
	sep2 := sky.Separation(a, c)
	testutil.AssertNear(t, "Separation exactly 180", sep2.Degrees(), 180, 1e-12)
}

func TestAirmassEdge(t *testing.T) {
	// Altitude exactly 0
	am, err := sky.Airmass(angle.Deg(0))
	testutil.AssertNoError(t, err)
	if am < 30 || am > 40 {
		t.Errorf("Airmass(0) = %v, expected ~38 (hard-coded limit)", am)
	}

	// Altitude slightly below 0
	_, err = sky.Airmass(angle.Deg(-0.1))
	if err != sky.ErrBelowHorizon {
		t.Errorf("Airmass(-0.1) expected ErrBelowHorizon, got %v", err)
	}
}

func TestSkyCoordinates(t *testing.T) {
	geo, _ := earth.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := observatory.NewSite("Equator", geo, angle.Zero(), nil)
	tm := time.FromJD(2451545.0, time.TT) // J2000
	target := coord.ICRS{RA: angle.Deg(0), Dec: angle.Deg(0)}

	altAz, err := sky.AltAz(target, tm, site)
	testutil.AssertNoError(t, err)

	ha, err := sky.HourAngle(target, tm, site)
	testutil.AssertNoError(t, err)

	// Ensure ha is a valid angle
	if math.IsNaN(ha.Radians()) {
		t.Errorf("HourAngle is NaN")
	}

	zd := sky.ZenithDistance(altAz.Alt)
	testutil.AssertNear(t, "ZenithDistance", zd.Degrees(), 90.0-altAz.Alt.Degrees(), 1e-12)

	// Boundary Condition: Poles
	npGeo, _ := earth.NewGeodetic(angle.Zero(), angle.Deg(90), 0)
	npSite, _ := observatory.NewSite("NorthPole", npGeo, angle.Zero(), nil)
	npTarget := coord.ICRS{RA: angle.Deg(180), Dec: angle.Deg(90)}
	npAltAz, err := sky.AltAz(npTarget, tm, npSite)
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "NorthPole Zenith Alt", npAltAz.Alt.Degrees(), 90, 1e-2)

	spGeo, _ := earth.NewGeodetic(angle.Zero(), angle.Deg(-90), 0)
	spSite, _ := observatory.NewSite("SouthPole", spGeo, angle.Zero(), nil)
	spTarget := coord.ICRS{RA: angle.Deg(180), Dec: angle.Deg(-90)}
	spAltAz, err := sky.AltAz(spTarget, tm, spSite)
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "SouthPole Zenith Alt", spAltAz.Alt.Degrees(), 90, 1e-2)
}
