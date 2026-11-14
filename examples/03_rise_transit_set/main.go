// Package main demonstrates rise, transit, and set computation.
package main

import (
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

func main() {
	// This example runs offline, so it uses no Earth-orientation data
	// (UT1 = UTC, no polar motion). That can move a rise or set by up to
	// about a second, below the minute these times are read to. For full
	// accuracy, blank-import remote/eop and consent to the IERS bulletin, as
	// 09_geometry_events does.
	time.RegisterModel(time.ZeroModel{})

	// 1. Setup Observatory (Quinta Calixto, Brazil)
	site, err := plan.NewSiteEarthLocation("Quinta Calixto", -22.528478, -46.473002, 835.05)
	if err != nil {
		log.Fatalf("site: %v", err)
	}

	// 2. The target: Sirius, with its J2000 position, proper motion and
	// parallax from SIMBAD. Proper motion matters for a star this close:
	// Sirius moves 1.3 arcsec a year. 05_resolve_name shows how to look a
	// name up in a catalog instead.
	target := plan.NewStar("Sirius", angle.Deg(101.287155), angle.Deg(-16.716116),
		plan.WithProperMotion(angle.Arcsec(-0.54601), angle.Arcsec(-1.22307)),
		plan.WithParallax(angle.Arcsec(0.37921)),
		plan.WithStarMagnitude(-1.46),
	)

	// 3. Define the Time interval (24 hours starting at 6 PM local time)
	tz, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		log.Fatalf("tz: %v", err)
	}

	start := time.Date(2026, 4, 6, 18, 0, 0, 0, tz)
	end := start.Add(unit.Hours(24))

	// 4. Find Rise/Set/Transit events.
	// The threshold is computed automatically from the site's elevation,
	// accounting for standard atmospheric refraction and horizon dip.
	events, err := plan.VisibilityEvents(start, end, target, site)
	if err != nil {
		log.Fatalf("events: %v", err)
	}

	fmt.Printf("Events for %s from %s over 24 hours from %s:\n\n",
		target.Name(), site.Name(), start.Format("2006-01-02 15:04 MST"))

	for _, e := range events {
		fmt.Printf("- %-10s at %s  (Alt=%s, Az=%s)\n", e.Kind, e.Time.Format("2006-01-02 15:04:05 MST"), e.Altitude.DMSString(0), e.Azimuth.DMSString(0))
	}
}
