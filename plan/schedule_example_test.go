package plan

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/unit"
)

func ExampleScheduler_BuildSchedule() {
	// Setup observatory and planner constraints
	site, _ := NewSite("Greenwich", angle.LatDeg(0), angle.LonDeg(0), unit.Meters(0))
	planner, _ := NewPlanner(site, []Constraint{Altitude{Threshold: angle.Deg(30)}})

	// Configure transition overheads (slew speeds, filter changes)
	transition := &BasicTransitionModel{
		BaseSetup:           unit.Minutes(1),
		SlewRate:            unit.DegreesPerSec(2.0), // degrees per second
		FilterChangePenalty: unit.Seconds(30),
	}

	scheduler := NewScheduler(planner, &PriorityStrategy{Step: unit.Minutes(5)}, transition)

	// Define our observing blocks using Custom coordinates
	blocks := []*Block{
		{
			ID:       "BlockA",
			Target:   NewStar("TargetA", angle.Hour(18.69), angle.Zero()),
			Duration: unit.Minutes(30),
			Priority: 2.0,
			Config:   Configuration{Filter: "V"},
			Cadence: &Cadence{
				MinInterval: unit.Hours(2),
				Repeats:     1, // Observe 2 times total
			},
		},
		{
			ID:       "BlockB",
			Target:   NewStar("TargetB", angle.Hour(18.69), angle.Deg(45)),
			Duration: unit.Minutes(45),
			Priority: 1.0,
			Config:   Configuration{Filter: "R"},
		},
	}

	// Generate a 6-hour schedule
	start := epoch.FromJD(2451545.0, epoch.UTC) // J2000 Noon
	window := Window{Start: start, End: start.Add(unit.Hours(6))}

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
