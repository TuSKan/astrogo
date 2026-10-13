package plan_test

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestDetailsRiseAndSetAreTheEventAPIs is #704: GetDetails searched a
// geometric 0° horizon of its own, so the Sun's rise and set in it were 5 to
// 7 minutes from SunEvents', the Moon's 5.5 to 8 from MoonEvents' and a
// star's 4.1 from VisibilityEvents', measured at London over 2026. It also
// reported the first rise after twelve hours before the Context's time,
// often a pass already over.
//
// Now each is the event API's own rise and set for the pass at the
// Context's time: the one in progress when the target is up, the next one
// when it is down. The instants come from the same solver on the same
// horizon over a different window, so they agree to its 1 s tolerance; 2 s
// allows for the two bisections starting from different samples. The
// instants step 4 h 7 min across three days, so every target is caught up and
// down, before and after its culmination.
func TestDetailsRiseAndSetAreTheEventAPIs(t *testing.T) {
	t.Parallel()

	prov := eph.Default()

	loc, err := coord.NewGeodetic(angle.Deg(-0.1278), angle.Deg(51.5074), 0)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	site, err := plan.NewSite("London", loc)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	sirius := plan.NewStar("Sirius", angle.Deg(101.2875), angle.Deg(-16.7161))

	for _, c := range []struct {
		name   string
		target plan.Observable
		events func(start, end time.Time) ([]plan.Event, error)
	}{
		{"Sun", plan.NewSun(prov), func(s, e time.Time) ([]plan.Event, error) { return plan.SunEvents(s, e, site, prov) }},
		{"Moon", plan.NewMoon(prov), func(s, e time.Time) ([]plan.Event, error) { return plan.MoonEvents(s, e, site, prov) }},
		{"Sirius", sirius, func(s, e time.Time) ([]plan.Event, error) { return plan.VisibilityEvents(s, e, sirius, site) }},
	} {
		start := time.Date(2026, time.March, 3, 0, 0, 0, 0, time.LocationUTC)
		end := start.Add(unit.Days(3))
		up, down := 0, 0

		// One search covers every instant's pass: a day before the first,
		// two after the last, the window GetDetails searches around each.
		events, err := c.events(start.Add(unit.Hours(-24)), end.Add(unit.Hours(48)))
		if err != nil {
			t.Fatalf("%s: events: %v", c.name, err)
		}

		for at := start; at.Before(end); at = at.Add(unit.Minutes(247)) {
			d, err := c.target.GetDetails(coord.NewContext(at, loc, atmosphere.Refraction{}), plan.DetailOverrides{})
			if err != nil {
				t.Fatalf("%s at %v: GetDetails: %v", c.name, at, err)
			}

			// The pass at the instant, from the event API's own list: the
			// last rise or set before it says whether the target is up.
			var last, rise, set *plan.Event

			for i := range events {
				if e := &events[i]; e.Time.Before(at) && (e.Kind == plan.EventRise || e.Kind == plan.EventSet) {
					last = e
				}
			}

			if last != nil && last.Kind == plan.EventRise {
				rise = last
				up++
			} else {
				down++
			}

			for i := range events {
				e := &events[i]

				switch {
				case rise == nil && e.Kind == plan.EventRise && !e.Time.Before(at):
					rise = e
				case rise != nil && set == nil && e.Kind == plan.EventSet && e.Time.After(rise.Time):
					set = e
				}
			}

			if rise == nil || set == nil {
				t.Fatalf("%s at %v: the event API has no rise and set around it; the fixture no longer covers it", c.name, at)
			}

			for _, f := range []struct {
				what      string
				got, want *time.Time
			}{{"rise", d.RiseTime, &rise.Time}, {"set", d.SetTime, &set.Time}} {
				switch {
				case f.got == nil:
					t.Errorf("%s at %v: GetDetails has no %s, the event API's is %v", c.name, at, f.what, *f.want)
				case f.got.Sub(*f.want).Abs() > unit.Seconds(2):
					t.Errorf("%s at %v: GetDetails' %s is %v, the event API's %v (%v apart)",
						c.name, at, f.what, *f.got, *f.want, f.got.Sub(*f.want))
				}
			}
		}

		if up == 0 || down == 0 {
			t.Errorf("%s: up at %d instants and down at %d; the fixture must catch both", c.name, up, down)
		}
	}
}
