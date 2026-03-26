package plan_test

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constraint"
	"github.com/TuSKan/astrogo/earth"
	"github.com/TuSKan/astrogo/observatory"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/sky"
	"github.com/TuSKan/astrogo/time"
)

func ExamplePlanner_Observable() {
	// Setup observatory
	loc, _ := earth.NewGeodetic(angle.Deg(-70), angle.Deg(-30), 2400) // Chile
	site, _ := observatory.NewSite("Paranal", loc, angle.Zero(), nil)

	// Constraints
	constraints := []constraint.Constraint{
		constraint.MinAltitudeConstraint{MinAlt: angle.Deg(30)},
	}

	planner, err := plan.NewPlanner(site, constraints)
	if err != nil {
		panic(err)
	}

	// Target
	obj := sky.NewTarget("Arp 220", 233.738, 23.503)
	t := time.NowUTC()

	visible, _ := planner.Observable(obj, t)
	if visible {
		fmt.Println("Target is visible!")
	} else {
		fmt.Println("Target is not visible.")
	}
	// Output: Target is not visible.
}
