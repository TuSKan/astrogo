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
	"github.com/TuSKan/astrogo/examples/internal/show"
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

	show.Title("Telescope slew estimate", site.Name()+", "+t.Format("2006-01-02 15:04 MST"))

	tab := show.NewTable(show.Left(""), show.Left("Target"), show.Left("Filter"), show.Right("Altitude"), show.Right("Azimuth"))
	tab.Row("from", blockA.Target.Name(), blockA.Config.Filter,
		fmt.Sprintf("%.1f°", tr.FromAltAz.Alt().Degrees()), fmt.Sprintf("%.1f°", tr.FromAltAz.Az().Degrees()))
	tab.Row("to", blockB.Target.Name(), blockB.Config.Filter,
		fmt.Sprintf("%.1f°", tr.ToAltAz.Alt().Degrees()), fmt.Sprintf("%.1f°", tr.ToAltAz.Az().Degrees()))

	fmt.Println()
	tab.Print()

	show.Section("What the move costs")
	show.Fields(
		show.KV("Axis moves", "altitude %.1f°, azimuth %.1f°", dAlt, dAz),
		show.KV("Slew", "%s sets it: %.1f° at %.1f°/s = %.0f s", slowest, move, model.SlewRate.DegreesPerSec(), slew.Seconds()),
		show.KV("Filter change", "%s to %s: %.0f s", blockA.Config.Filter, blockB.Config.Filter, model.FilterChangePenalty.Seconds()),
		show.KV("Total", "%.0f s", total.Seconds()),
	)
	fmt.Println()
	show.Note("The night's first block has nothing to slew from, and is charged",
		fmt.Sprintf("BaseSetup (%.0f s) instead.", model.BaseSetup.Seconds()))
}
