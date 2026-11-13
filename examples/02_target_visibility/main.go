// Package main demonstrates target visibility analysis: is a target
// observable from a site at a given instant, and if not, why not.
package main

import (
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	// This example runs offline, so it uses no Earth-orientation data
	// (UT1 = UTC, no polar motion). That can move an alt/az position by up
	// to 13.5 arcsec, which decides nothing against a 30° limit. For full
	// accuracy, blank-import remote/eop and consent to the IERS bulletin, as
	// 05_resolve_name does.
	time.RegisterModel(time.ZeroModel{})

	// 1. Setup Observatory (Quinta Calixto, Brazil)
	site, err := plan.NewSiteEarthLocation("Quinta Calixto", -22.528478, -46.473002, 835.05)
	if err != nil {
		log.Fatalf("site: %v", err)
	}

	// 2. Define Constraints: The target must be above 30 degrees altitude
	constraints := []plan.Constraint{
		plan.Altitude{Threshold: angle.Deg(30)},
	}

	// 3. The target: the Orion Nebula, at its J2000 position from SIMBAD.
	// 05_resolve_name shows how to look a name up in a catalog instead.
	target := plan.NewDeepSkyObject("Orion Nebula (M42)", angle.Deg(83.8201), angle.Deg(-5.3876))

	// 4. Two instants on the same evening (UTC-3): early, with Orion high
	// in the west, and late, after it has set.
	tz, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		log.Fatalf("tz: %v", err)
	}

	for _, tm := range []time.Time{
		time.Date(2026, 4, 6, 19, 0, 0, 0, tz),
		time.Date(2026, 4, 6, 23, 0, 0, 0, tz),
	} {
		// 5. Check Visibility! IsObservable reports each constraint's result,
		// so a "no" comes with its reason. (plan.Planner.Observable gives the
		// yes or no alone.)
		eval, err := plan.IsObservable(target, tm, site, constraints...)
		if err != nil {
			log.Fatalf("visibility: %v", err)
		}

		verdict := "not observable"
		if eval.Observable {
			verdict = "observable"
		}

		fmt.Printf("%s from %s at %s: %s\n", target.Name(), site.Name(), tm.Format("2006-01-02 15:04 -0700"), verdict)
		fmt.Printf("  altitude %.1f°, azimuth %.1f°\n", eval.AltAz.Alt().Degrees(), eval.AltAz.Az().Degrees())

		for _, r := range eval.Results {
			fmt.Printf("  %s\n", r)
		}

		fmt.Println()
	}
}
