package plan_test

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constraint"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/earth"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/observatory"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/sky"
	"github.com/TuSKan/astrogo/time"
)

func TestPlanner(t *testing.T) {
	loc, _ := earth.NewGeodetic(angle.Deg(0), angle.Deg(0), 0)
	site, _ := observatory.NewSite("Test", loc, angle.Deg(0), nil)
	constraints := []constraint.Constraint{
		constraint.MinAltitudeConstraint{MinAlt: angle.Deg(30)},
	}

	planner, err := plan.NewPlanner(site, constraints)
	testutil.AssertNoError(t, err)
	tm := time.NowUTC()

	objs := []sky.Object{
		&sky.Target{Name: "High", Coord: coord.ICRS{RA: angle.Deg(0), Dec: angle.Deg(45)}},
		&sky.Target{Name: "Low", Coord: coord.ICRS{RA: angle.Deg(180), Dec: angle.Deg(-45)}},
	}

	filtered, err := planner.FilterObservable(objs, tm)
	testutil.AssertNoError(t, err)

	if len(filtered) != 1 {
		t.Errorf("expected 1 observable object, got %d", len(filtered))
	}
}
