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

	// 2. Define our Transition Model representing the telescope's physical properties
	model := &plan.BasicTransitionModel{
		BaseSetup:           30 * time.Second, // Time to settle and expose
		SlewRate:            1.5,              // The telescope moves 1.5 degrees per second
		FilterChangePenalty: 45 * time.Second, // Time to change telescope filter
	}

	// 3. Define two Observing Blocks
	// Block A (Looking North)
	blockA := plan.Block{
		Target: plan.Custom{Coord: coord.NewICRS(angle.Hour(10.0), angle.Deg(20.0))},
		Config: plan.Configuration{Filter: "V"},
	}

	// Block B (Looking South, changing filter)
	blockB := plan.Block{
		Target: plan.Custom{Coord: coord.NewICRS(angle.Hour(12.0), angle.Deg(-40.0))},
		Config: plan.Configuration{Filter: "R"},
	}

	// 4. Time of Slew
	t := time.NowUTC()

	// 5. Estimate Transition Time from Block A to Block B
	ctx := plan.TransitionContext{
		FromBlock: &blockA,
		ToBlock:   &blockB,
		FromTime:  t,
		ToTime:    t,
		Site:      site,
	}
	transitionTime, err := model.Overhead(ctx)
	if err != nil {
		fmt.Printf("Calculation error: %v\n", err)
		return
	}

	fmt.Println("Telescope Transition Slew Estimation")
	fmt.Println("====================================")
	fmt.Printf("Base Setup Time:        %.0f seconds\n", model.BaseSetup.Seconds())
	fmt.Printf("Filter Change (V -> R): %.0f seconds\n", model.FilterChangePenalty.Seconds())
	fmt.Printf("Total Slew & Setup:     %.0f seconds\n", transitionTime.Seconds())
}
