// Package main demonstrates telescope slew time calculation.
package main

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	// 1. Setup Observatory (São Paulo, Brazil)
	loc, _ := coord.NewEarthLocation(-23.5505, -46.6333, 760)
	site, _ := plan.NewSite("São Paulo", loc, 0, nil)

	// 2. Define our Transition Model representing the telescope's physical properties
	model := &plan.BasicTransitionModel{
		BaseSetup:           30 * time.Second, // Time to settle and expose
		SlewRate:            1.5,              // The telescope moves 1.5 degrees per second
		FilterChangePenalty: 45 * time.Second, // Time to change telescope filter
	}

	// 3. Define two Observing Blocks
	// Block A (Looking North)
	blockA := plan.Block{
		Target: plan.NewStar("TargetA", angle.Hour(10.0), angle.Deg(20.0)),
		Config: plan.Configuration{Filter: "V"},
	}

	// Block B (Looking South, changing filter)
	blockB := plan.Block{
		Target: plan.NewStar("TargetB", angle.Hour(12.0), angle.Deg(-40.0)),
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
