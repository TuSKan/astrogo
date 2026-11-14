// Package main demonstrates telescope slew time calculation.
//
// plan.BasicTransitionModel charges for moving between two observations:
// the slower of the two mount axes at the slew rate, plus a filter change
// when the filter differs. This example shows each part.
package main

import (
	"fmt"
	"log"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

func main() {
	// This example runs offline, so it uses no Earth-orientation data
	// (UT1 = UTC, no polar motion). That can move an alt/az position by up
	// to 13.5 arcsec, which changes a slew time here by well under 0.1 s.
	// For full accuracy, blank-import remote/eop and consent to the IERS
	// bulletin, as 09_geometry_events does.
	time.RegisterModel(time.ZeroModel{})

	// 1. Setup Observatory (Quinta Calixto, Brazil)
	site, err := plan.NewSiteEarthLocation("Quinta Calixto", -22.528478, -46.473002, 835.05)
	if err != nil {
		log.Fatalf("site: %v", err)
	}

	// 2. The telescope's physical properties
	model := &plan.BasicTransitionModel{
		BaseSetup:           30 * time.Second,        // the night's first pointing, with nothing to slew from
		SlewRate:            unit.DegreesPerSec(1.5), // each axis moves 1.5 degrees per second
		FilterChangePenalty: 45 * time.Second,        // time to change filter
	}

	// 3. Two observing blocks, in different parts of the sky and filters
	blockA := plan.Block{
		Target: plan.NewStar("Target A", angle.Hour(10.0), angle.Deg(20.0)),
		Config: plan.Configuration{Filter: "V"},
	}

	blockB := plan.Block{
		Target: plan.NewStar("Target B", angle.Hour(12.0), angle.Deg(-40.0)),
		Config: plan.Configuration{Filter: "R"},
	}

	// 4. When the telescope moves. A fixed instant keeps the output the same
	// on every run; time.NowUTC() would estimate the move right now.
	tz, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		log.Fatalf("tz: %v", err)
	}

	t := time.Date(2026, 4, 6, 21, 0, 0, 0, tz)

	// 5. Where each target is in the sky, then what the model charges to
	// move between them.
	tr, err := plan.NewTransition(&blockA, &blockB, t, t, site)
	if err != nil {
		log.Fatalf("transition: %v", err)
	}

	total, err := model.Overhead(tr)
	if err != nil {
		log.Fatalf("overhead: %v", err)
	}

	// The model moves both axes at once, so the longer move sets the time.
	dAlt := math.Abs(tr.ToAltAz.Alt().Degrees() - tr.FromAltAz.Alt().Degrees())

	dAz := math.Abs(tr.ToAltAz.Az().Degrees() - tr.FromAltAz.Az().Degrees())
	if dAz > 180 {
		dAz = 360 - dAz
	}

	slowest, move := "altitude", dAlt
	if dAz > dAlt {
		slowest, move = "azimuth", dAz
	}

	slew := total - model.FilterChangePenalty

	fmt.Printf("Telescope slew estimate, %s\n\n", t.Format("2006-01-02 15:04 MST"))
	fmt.Printf("From  %-9s (%s)   alt %5.1f°   az %5.1f°\n",
		blockA.Target.Name(), blockA.Config.Filter, tr.FromAltAz.Alt().Degrees(), tr.FromAltAz.Az().Degrees())
	fmt.Printf("To    %-9s (%s)   alt %5.1f°   az %5.1f°\n\n",
		blockB.Target.Name(), blockB.Config.Filter, tr.ToAltAz.Alt().Degrees(), tr.ToAltAz.Az().Degrees())
	fmt.Printf("Axis moves      altitude %.1f°, azimuth %.1f°\n", dAlt, dAz)
	fmt.Printf("Slew            %s sets it: %.1f° at %.1f°/s = %.0f s\n",
		slowest, move, model.SlewRate.DegreesPerSec(), slew.Seconds())
	fmt.Printf("Filter change   %s -> %s: %.0f s\n",
		blockA.Config.Filter, blockB.Config.Filter, model.FilterChangePenalty.Seconds())
	fmt.Printf("Total           %.0f s\n\n", total.Seconds())
	fmt.Printf("The night's first block has nothing to slew from, and is charged\n")
	fmt.Printf("BaseSetup (%.0f s) instead.\n", model.BaseSetup.Seconds())
}
