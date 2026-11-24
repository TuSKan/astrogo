// Package main is the README's first example: where Mars is tonight from
// a site in Brazil, and when it rises, transits and sets.
package main

import (
	"fmt"
	"log"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

func main() {
	site, err := plan.NewSite("Quinta Calixto", angle.LatDeg(-22.528478), angle.LonDeg(-46.473002), unit.Meters(835.05))
	if err != nil {
		log.Fatalf("site: %v", err)
	}

	mars := plan.NewMars(ephemeris.Default())

	tonight := time.Date(2026, 4, 15, 22, 0, 0, 0, time.LocationUTC)

	eval, err := plan.IsObservable(mars, tonight, site, plan.Altitude{Threshold: angle.Deg(30)})
	if err != nil {
		log.Fatalf("eval: %v", err)
	}

	events, err := plan.VisibilityEvents(tonight, tonight.Add(unit.Days(1)), mars, site)
	if err != nil {
		log.Fatalf("events: %v", err)
	}

	fmt.Printf("Mars right now: altitude %.1f°, observable above 30°: %v\n",
		eval.AltAz.Alt().Degrees(), eval.Observable)

	for _, e := range events {
		fmt.Printf("  %-8s %s\n", e.Kind, e.Time.Format("2006-01-02 15:04 MST"))
	}
}
