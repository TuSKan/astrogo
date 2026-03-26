package sky_test

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/sky"
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
