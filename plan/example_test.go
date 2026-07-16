package plan

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"

	"github.com/TuSKan/astrogo/coord"

	"github.com/TuSKan/astrogo/time"
)

func ExamplePlanner_Observable() {
	// Setup observatory
	loc, _ := coord.NewGeodetic(angle.Deg(-70), angle.Deg(-30), 2400) // Chile
	site, _ := NewSite("Paranal", loc, nil)

	// Constraints
	constraints := []Constraint{
		Altitude{Threshold: angle.Deg(30)},
	}

	planner, err := NewPlanner(site, constraints)
	if err != nil {
		panic(err)
	}

	// Target
	obj := NewStar("Arp 220", angle.Deg(233.738), angle.Deg(23.503))

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
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := NewSite("Greenwich", loc, nil)

	// Target at Zenith initially (J2000 Noon LST ~18.69h)
	obj := NewStar("ZenithTarget", angle.Hour(18.69), angle.Zero())

	start := time.FromJD(2451545.0, time.UTC) // J2000 Noon
	end := start.Add(6 * time.Hour)
	step := 10 * time.Minute // ≤ 15min

	// Constraint: Altitude > 30 degrees
	constraints := []Constraint{
		Altitude{Threshold: angle.Deg(30)},
	}

	windows, _ := ObservableWindows(obj, start, end, step, site, constraints...)

	for _, w := range windows {
		fmt.Printf("Window: %s to %s\n", w.Start.Format("2006-01-02 15:04:05"), w.End.Format("2006-01-02 15:04:05"))
	}
	// Output: Window: 2000-01-01 12:00:00 to 2000-01-01 15:58:59
}

func ExampleRankObservables() {
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := NewSite("Test", loc, nil)
	tm := time.FromJD(2451545.0, time.UTC) // J2000 Noon

	obj1 := NewStar("NearZenith", angle.Hour(18.69), angle.Deg(0))
	obj2 := NewStar("Lower", angle.Hour(18.69), angle.Deg(45))
	objs := []Observable{obj1, obj2}

	ranked, _ := RankObservables(objs, tm, site)

	for i, rt := range ranked {
		fmt.Printf("%d. %-10s (Score: %5.1f)\n", i+1, rt.Object.Name(), rt.Score)
	}
	// Output:
	// 1. NearZenith (Score:  67.5)
	// 2. Lower      (Score:  45.4)
}
