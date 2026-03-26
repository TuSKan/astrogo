package plan_test

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/constraint"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/earth"
	"github.com/TuSKan/astrogo/observatory"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/target"
	"github.com/TuSKan/astrogo/time"
)

func ExamplePlanner_Observable() {
	// Setup observatory
	loc, _ := earth.NewGeodetic(angle.Deg(-70), angle.Deg(-30), 2400) // Chile
	site, _ := observatory.NewSite("Paranal", loc, angle.Zero(), nil)

	// Constraints
	constraints := []constraint.Constraint{
		constraint.Altitude{Threshold: angle.Deg(30)},
	}

	planner, err := plan.NewPlanner(site, constraints)
	if err != nil {
		panic(err)
	}

	// Target
	obj := target.NewFixed(catalog.Target{
		Name: "Arp 220",
		Coord: coord.ICRS{
			RA:  angle.Deg(233.738),
			Dec: angle.Deg(23.503),
		},
	})

	// Fixed time: Equinox 2000 midnight (T=0.5).
	t := time.FromJD(2451545.5, time.UTC)

	visible, _ := planner.Observable(obj, t)
	if visible {
		fmt.Println("Target is visible!")
	} else {
		fmt.Println("Target is not visible.")
	}
	// Output: Target is not visible.
}

func ExampleObservableWindows() {
	loc, _ := earth.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := observatory.NewSite("Greenwich", loc, angle.Zero(), nil)

	// Target at Zenith initially (J2000 Noon LST ~18.69h)
	obj := target.NewFixed(catalog.Target{
		Name:  "ZenithTarget",
		Coord: coord.ICRS{RA: angle.Hour(18.69), Dec: angle.Zero()},
	})

	start := time.FromJD(2451545.0, time.UTC) // J2000 Noon
	end := start.Add(6 * time.Hour)
	step := 1 * time.Hour

	// Constraint: Altitude > 30 degrees
	constraints := []constraint.Constraint{
		constraint.Altitude{Threshold: angle.Deg(30)},
	}

	windows, _ := plan.ObservableWindows(obj, start, end, step, site, constraints...)

	for _, w := range windows {
		fmt.Printf("Window: %s to %s\n", w.Start, w.End)
	}
	// Output: Window: JD 2451545.00000000 (UTC) to JD 2451545.16666667 (UTC)
}

func ExampleRankObservables() {
	loc, _ := earth.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := observatory.NewSite("Test", loc, angle.Zero(), nil)
	tm := time.FromJD(2451545.0, time.UTC) // J2000 Noon

	objs := []target.Observable{
		target.NewFixed(catalog.Target{Name: "NearZenith", Coord: coord.ICRS{RA: angle.Hour(18.69), Dec: angle.Deg(0)}}),
		target.NewFixed(catalog.Target{Name: "Lower", Coord: coord.ICRS{RA: angle.Hour(18.69), Dec: angle.Deg(45)}}),
	}

	ranked, _ := plan.RankObservables(objs, tm, site)

	for i, rt := range ranked {
		fmt.Printf("%d. %-10s (Score: %5.1f)\n", i+1, rt.Object.Name(), rt.Score)
	}
	// Output:
	// 1. NearZenith (Score:  89.9)
	// 2. Lower      (Score:  45.0)
}
