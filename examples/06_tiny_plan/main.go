package main

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	// 1. Setup Observatory (São Paulo, Brazil with precise coordinates from user's app)
	// Lat: 23° 36' 03'' S = -23.600833°
	// Lon: 46° 39' 09'' W = -46.6525°
	// Elev: 786m
	loc, _ := coord.NewGeodetic(angle.Deg(-46.6525), angle.Deg(-23.600833), 786)
	site, _ := plan.NewSite("São Paulo", loc, angle.Zero(), nil)

	// Ensure targets are at least 20 degrees above the horizon
	planner, _ := plan.NewPlanner(site, []plan.Constraint{plan.Altitude{Threshold: angle.Deg(20)}})

	// 2. Configure Transition Overheads
	transition := &plan.BasicTransitionModel{
		BaseSetup:           1 * time.Minute,
		SlewRate:            2.0, // degrees per second
		FilterChangePenalty: 30 * time.Second,
	}

	// Create a scheduler using a simple priority-based strategy checking every 5 mins
	scheduler := plan.NewScheduler(planner, &plan.PriorityStrategy{Step: 5 * time.Minute}, transition)

	// 3. Define the Observing Blocks (Our "Plan")
	blocks := []*plan.Block{
		{
			ID:       "Alpha Centauri",
			Target:   plan.Custom{Label: "Alpha Cen", Coord: coord.NewICRS(angle.Hour(14.66), angle.Deg(-60.83))},
			Duration: 30 * time.Minute,
			Priority: 2.0, // High priority
			Config:   plan.Configuration{Filter: "V"},
		},
		{
			ID:       "Omega Centauri",
			Target:   plan.Custom{Label: "Omega Cen", Coord: coord.NewICRS(angle.Hour(13.44), angle.Deg(-47.47))},
			Duration: 45 * time.Minute,
			Priority: 1.0, // Lower priority
			Config:   plan.Configuration{Filter: "R"},
		},
	}

	// 4. Generate a schedule for tonight (starting at 7 PM for 6 hours)
	tz, _ := time.LoadLocation("America/Sao_Paulo")

	start := time.Date(2026, 4, 6, 19, 0, 0, 0, tz)
	window := plan.Window{Start: start, End: start.Add(6 * time.Hour)}

	schedule, err := scheduler.BuildSchedule(window, blocks)
	if err != nil {
		fmt.Printf("Failed to generate schedule: %v\n", err)
		return
	}

	// 5. Print out the generated timeline
	fmt.Println("Observing Schedule:")
	fmt.Println("===================")
	for i, b := range schedule.Blocks {
		fmt.Printf("%d. %-15s | Start: %s | Setup: %ds\n",
			i+1,
			b.Block.ID,
			b.Window.Start.ToGo().In(tz).Format("15:04"),
			int(b.SetupTime.Seconds()))
	}
}
