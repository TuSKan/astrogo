package plan

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

func ExampleScheduler_BuildSchedule() {
	// 1. Setup Observatory
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, _ := NewSite("Greenwich", loc, angle.Zero(), nil)

	// 2. Setup Planner and constraints
	constraints := []Constraint{
		Altitude{Threshold: angle.Deg(30)},
	}
	planner, _ := NewPlanner(site, constraints)

	// 3. Define Slew and Transition Model
	transition := &BasicTransitionModel{
		BaseSetup:           1 * time.Minute,
		SlewRate:            2.0, // degrees per second
		FilterChangePenalty: 30 * time.Second,
	}

	// 4. Create Strategy and Scheduler
	strategy := &PriorityStrategy{Step: 5 * time.Minute}
	scheduler := NewScheduler(planner, strategy, transition)

	// 5. Define Observation Blocks
	t1 := NewFixed(catalog.Target{Name: "TargetA", Coord: coord.NewICRS(angle.Hour(18.69), angle.Deg(0))})
	t2 := NewFixed(catalog.Target{Name: "TargetB", Coord: coord.NewICRS(angle.Hour(18.69), angle.Deg(45))})

	blocks := []*Block{
		{
			ID:       "BlockA",
			Target:   t1,
			Duration: 30 * time.Minute,
			Priority: 2.0, // Higher priority
			Config:   Configuration{Filter: "V"},
			Cadence: &Cadence{
				MinInterval: 2 * time.Hour,
				Repeats:     1, // Observe 2 times total
			},
		},
		{
			ID:       "BlockB",
			Target:   t2,
			Duration: 45 * time.Minute,
			Priority: 1.0,
			Config:   Configuration{Filter: "R"},
		},
	}

	// 6. Define Scheduling Window (e.g., 6 hours)
	start := time.FromJD(2451545.0, time.UTC) // J2000 Noon (Targets at ideal positions)
	window := Window{Start: start, End: start.Add(6 * time.Hour)}

	// 7. Generate Schedule
	schedule, _ := scheduler.BuildSchedule(window, blocks)

	// Print Results
	fmt.Printf("Generated Schedule for %s:\n", schedule.Site.Name())
	for i, b := range schedule.Blocks {
		fmt.Printf("%d. %s [%s] -> Start: %s | Setup: %ds\n",
			i+1, b.Block.ID, b.Block.Target.Name(), b.Window.Start, int(b.SetupTime.Seconds()))
	}

	if len(schedule.Unscheduled) > 0 {
		fmt.Println("\nUnscheduled Blocks:")
		for _, b := range schedule.Unscheduled {
			fmt.Printf("- %s: %s\n", b.Block.ID, b.Reason)
		}
	}
	
	// Output:
	// Generated Schedule for Greenwich:
	// 1. BlockA [TargetA] -> Start: JD 2451545.00069444 (UTC) | Setup: 60s
	// 2. BlockB [TargetB] -> Start: JD 2451545.02234890 (UTC) | Setup: 70s
	// 3. BlockA [TargetA] -> Start: JD 2451545.10554628 (UTC) | Setup: 59s
}
