package constraint_test

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constraint"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/earth"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/observatory"
	"github.com/TuSKan/astrogo/sky"
	"github.com/TuSKan/astrogo/time"
)

func TestAltAzConstraint(t *testing.T) {
	// North Pole makes tests deterministic (Alt independent of LST for fixed Dec)
	loc, _ := earth.NewGeodetic(angle.Deg(0), angle.Deg(90), 0)
	site, _ := observatory.NewSite("NorthPole", loc, angle.Deg(0), nil)
	tm := time.NowUTC()

	minAlt := constraint.MinAltitudeConstraint{MinAlt: angle.Deg(30)}
	maxAm := constraint.MaxAirmassConstraint{MaxAirmass: 2.0}

	// 1. High Object (near Zenith at North Pole)
	objHigh := &sky.Target{Coord: coord.ICRS{RA: angle.Deg(0), Dec: angle.Deg(89)}}
	ctxHigh := constraint.NewContext(objHigh, tm, site, nil)

	ok, err := minAlt.Evaluate(ctxHigh)
	testutil.AssertNoError(t, err)
	if !ok {
		t.Error("MinAltitude failed for high object")
	}

	ok, err = maxAm.Evaluate(ctxHigh)
	testutil.AssertNoError(t, err)
	if !ok {
		t.Error("MaxAirmass failed for high object")
	}

	// 2. Low Object (far below horizon from North Pole)
	objLow := &sky.Target{Coord: coord.ICRS{RA: angle.Deg(0), Dec: angle.Deg(-45)}}
	ctxLow := constraint.NewContext(objLow, tm, site, nil)

	ok, err = minAlt.Evaluate(ctxLow)
	testutil.AssertNoError(t, err)
	if ok {
		t.Error("MinAltitude passed for low object")
	}

	ok, err = maxAm.Evaluate(ctxLow)
	testutil.AssertNoError(t, err)
	if ok {
		t.Error("MaxAirmass passed for low object")
	}
}

func TestEvaluateAll(t *testing.T) {
	loc, _ := earth.NewGeodetic(angle.Deg(0), angle.Deg(90), 0)
	site, _ := observatory.NewSite("NorthPole", loc, angle.Deg(0), nil)
	tm := time.NowUTC()
	constraints := []constraint.Constraint{
		constraint.MinAltitudeConstraint{MinAlt: angle.Deg(30)},
	}

	obj := &sky.Target{Coord: coord.ICRS{RA: angle.Deg(0), Dec: angle.Deg(89)}}
	ok, err := constraint.EvaluateAll(obj, tm, site, constraints)
	testutil.AssertNoError(t, err)
	if !ok {
		t.Error("EvaluateAll failed for visible object")
	}
}
