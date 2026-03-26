package transform_test

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/earth"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/transform"
)

func TestTransformNearPole(t *testing.T) {
	// Observatory at North Pole
	tm := time.NowUTC()

	// Star at zenith from North Pole
	locN, _ := earth.NewGeodetic(angle.Deg(0), angle.Deg(90), 0)
	starNorth := coord.ICRS{RA: angle.Deg(0), Dec: angle.Deg(89.9)}
	aaN, err := transform.ICRSToAltAz(starNorth, tm, locN)
	testutil.AssertNoError(t, err)
	if aaN.Alt.Degrees() < 89.0 {
		t.Fatalf("expected near-zenith altitude at North Pole, got %.6f deg", aaN.Alt.Degrees())
	}

	// Star at horizon from South Pole
	locS, _ := earth.NewGeodetic(angle.Deg(0), angle.Deg(-90), 0)
	starEq := coord.ICRS{RA: angle.Deg(0), Dec: angle.Deg(0)}
	aaS, err := transform.ICRSToAltAz(starEq, tm, locS)
	testutil.AssertNoError(t, err)
	testutil.AssertNear(t, "Altitude at S.Pole Horizon", aaS.Alt.Degrees(), 0, 0.5)
}

func TestTransformBoundaryRA(t *testing.T) {
	loc, _ := earth.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	tm := time.NowUTC()

	// Test RA 359.999 vs 0.001 should yield very similar results
	s1 := coord.ICRS{RA: angle.Deg(359.999), Dec: angle.Deg(45)}
	s2 := coord.ICRS{RA: angle.Deg(0.001), Dec: angle.Deg(45)}

	aa1, _ := transform.ICRSToAltAz(s1, tm, loc)
	aa2, _ := transform.ICRSToAltAz(s2, tm, loc)

	diff := aa1.Az.Sub(aa2.Az).WrapPi().Degrees()
	if diff > 0.1 {
		t.Errorf("RA wrap discontinuity: Az1=%v, Az2=%v, diff=%v", aa1.Az, aa2.Az, diff)
	}
}

func TestNegativeLongitude(t *testing.T) {
	// Lon -45 should be same as Lon 315
	loc1, _ := earth.NewGeodetic(angle.Deg(-45), angle.Deg(0), 0)
	loc2, _ := earth.NewGeodetic(angle.Deg(315), angle.Deg(0), 0)
	tm := time.NowUTC()
	star := coord.ICRS{RA: angle.Deg(0), Dec: angle.Deg(0)}

	aa1, _ := transform.ICRSToAltAz(star, tm, loc1)
	aa2, _ := transform.ICRSToAltAz(star, tm, loc2)

	testutil.AssertNear(t, "Alt same for -45/315 lon", aa1.Alt.Degrees(), aa2.Alt.Degrees(), 1e-10)
	diffAz := aa1.Az.Sub(aa2.Az).WrapPi().Degrees()
	testutil.AssertNear(t, "Az same for -45/315 lon", diffAz, 0, 1e-10)
}
