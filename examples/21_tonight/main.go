// Package main answers the first question an observer asks: what is tonight
// like? plan.TonightAt gives the night's sunset and sunrise, its civil,
// nautical and astronomical nights, and the Moon, in one call.
package main

import (
	"fmt"
	"log"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/examples/internal/show"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

func main() {
	// This example runs offline, so it uses no Earth-orientation data
	// (UT1 = UTC, no polar motion). That moves an event by at most about a
	// second, below the minute these times are read to. For full accuracy,
	// blank-import remote/eop and consent to the IERS bulletin, as
	// 09_geometry_events does.
	time.RegisterModel(time.ZeroModel{})

	// Two sites on two nights. Quinta Calixto in Brazil, in August, has every
	// kind of night. Greenwich at midsummer has none darker than nautical:
	// the Sun never gets 18° below its horizon.
	nights := []struct {
		name     string
		lat, lon float64
		height   float64
		zone     string
		year     int
		month    time.Month
		day      int
	}{
		{"Quinta Calixto", -22.528478, -46.473002, 835.05, "America/Sao_Paulo", 2026, time.August, 1},
		{"Greenwich", 51.4778, -0.0015, 46, "Europe/London", 2026, time.June, 21},
	}

	for _, n := range nights {
		site, err := plan.NewSiteEarthLocation(n.name, n.lat, n.lon, n.height)
		if err != nil {
			log.Fatalf("site: %v", err)
		}

		zone, err := time.LoadLocation(n.zone)
		if err != nil {
			log.Fatalf("time zone: %v", err)
		}

		// Local noon names that date's evening: TonightAt gives the night
		// under way at an instant, or the coming one in daylight.
		noon := time.Date(n.year, n.month, n.day, 12, 0, 0, 0, zone)

		tonight, err := plan.TonightAt(noon, site, eph.Default())
		if err != nil {
			log.Fatalf("tonight: %v", err)
		}

		printTonight(site.Name(), noon, tonight, zone)
	}
}

// printTonight shows one night, in the site's local time.
func printTonight(site string, date time.Time, t plan.Tonight, zone *time.Location) {
	clock := func(at time.Time) string { return at.In(zone).Format("15:04") }

	show.Title("Tonight at "+site, "the night of "+date.Format("2006-01-02"))

	tab := show.NewTable(show.Left("Night"), show.Right("Begins"), show.Right("Ends"))
	tab.Row("Sun below the horizon", clock(t.Sunset), clock(t.Sunrise))

	for _, k := range []struct {
		name  string
		night *plan.Night
	}{
		{"Civil (Sun 6° down)", t.Civil},
		{"Nautical (12° down)", t.Nautical},
		{"Astronomical (18° down)", t.Astronomical},
	} {
		// A nil night is one the Sun does not reach: no astronomical darkness
		// on a midsummer night at Greenwich.
		if k.night == nil {
			tab.Row(k.name, "none", "none")
			continue
		}

		tab.Row(k.name, clock(k.night.Dusk), clock(k.night.Dawn))
	}

	fmt.Println()
	tab.Print()
	fmt.Println()

	show.Fields(
		show.KV("Moon up at sunset", "%v", t.MoonUpAtSunset),
		show.KV("Moonrise", "%s", eventClock(t.Moonrise, clock)),
		show.KV("Moonset", "%s", eventClock(t.Moonset, clock)),
		show.KV("Moon illuminated", "%.0f%%", 100*t.MoonIllumination),
	)
	fmt.Println()
}

// eventClock is a Moon event's local time, or why there is none: the Moon
// may not rise or set between sunset and sunrise.
func eventClock(at *time.Time, clock func(time.Time) string) string {
	if at == nil {
		return "not tonight"
	}

	return clock(*at)
}
