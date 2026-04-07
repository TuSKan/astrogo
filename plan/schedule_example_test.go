package plan

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

func ExampleScheduler_BuildSchedule() {
	// Setup observatory and planner constraints
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := NewSite("Greenwich", loc, angle.Zero(), nil)
	planner, _ := NewPlanner(site, []Constraint{Altitude{Threshold: angle.Deg(30)}})

	// Configure transition overheads (slew speeds, filter changes)
	transition := &BasicTransitionModel{
		BaseSetup:           1 * time.Minute,
		SlewRate:            2.0, // degrees per second
		FilterChangePenalty: 30 * time.Second,
	}

	scheduler := NewScheduler(planner, &PriorityStrategy{Step: 5 * time.Minute}, transition)

	// Define our observing blocks using Custom coordinates
	blocks := []*Block{
		{
			ID:       "BlockA",
			Target:   Custom{Label: "TargetA", Coord: coord.NewICRS(angle.Hour(18.69), angle.Zero())},
			Duration: 30 * time.Minute,
			Priority: 2.0,
			Config:   Configuration{Filter: "V"},
			Cadence: &Cadence{
				MinInterval: 2 * time.Hour,
				Repeats:     1, // Observe 2 times total
			},
		},
		{
			ID:       "BlockB",
			Target:   Custom{Label: "TargetB", Coord: coord.NewICRS(angle.Hour(18.69), angle.Deg(45))},
			Duration: 45 * time.Minute,
			Priority: 1.0,
			Config:   Configuration{Filter: "R"},
		},
	}

	// Generate a 6-hour schedule
	start := time.FromJD(2451545.0, time.UTC) // J2000 Noon
	window := Window{Start: start, End: start.Add(6 * time.Hour)}

	schedule, _ := scheduler.BuildSchedule(window, blocks)

	// Print Results
	for _, b := range schedule.Blocks {
		fmt.Printf("%s [%s] -> Setup: %ds\n", b.Block.ID, b.Block.Target.Name(), int(b.SetupTime.Seconds()))
	}

	// Output:
	// BlockA [TargetA] -> Setup: 60s
	// BlockB [TargetB] -> Setup: 70s
	// BlockA [TargetA] -> Setup: 59s
}
