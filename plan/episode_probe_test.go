package plan

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestEpisodeJustAfterASetIsTheNextOne is #422's case: at Barcelona the Moon
// sets at 19:03:41 UTC on 2026-07-13, so the episode for a window starting
// at 19:04 is the next one, from the rise at 04:08:23 to the set at 19:50:30
// on the 14th. Those are Skyfield 1.55's instants (DE440s) for the Moon's
// apparent centre crossing the almanac horizon a generic target rises on,
// −0.5667° (#568); before #568 the threshold was 0° and the instants
// 18:59:49, 04:12:15 and 19:46:53. Episode's probe once measured a refracted
// altitude against the geometric threshold, called the Moon up, and returned
// the episode that had ended seconds before.
func TestEpisodeJustAfterASetIsTheNextOne(t *testing.T) {
	site, err := NewSiteEarthLocation("Barcelona", 41.39, 2.17, 0)
	if err != nil {
		t.Fatal(err)
	}

	from := time.Date(2026, 7, 13, 19, 4, 0, 0, time.LocationUTC)

	rise, set, err := Episode(from, from.Add(unit.Hours(10)), NewMoon(eph.Default()), site)
	if err != nil {
		t.Fatal(err)
	}

	if rise == nil || set == nil {
		t.Fatalf("Episode: rise %v, set %v; want both", rise, set)
	}

	for _, c := range []struct {
		what      string
		got, want time.Time
	}{
		{"rise", rise.Time, time.Date(2026, 7, 14, 4, 8, 23, 453e6, time.LocationUTC)},
		{"set", set.Time, time.Date(2026, 7, 14, 19, 50, 30, 236e6, time.LocationUTC)},
	} {
		if d := c.got.Sub(c.want).Abs(); d > unit.Seconds(2) {
			t.Errorf("%s %v, Skyfield gives %v (off by %v)", c.what, c.got, c.want, d)
		}
	}
}

// TestEpisodeJustBeforeARiseIsThatRise: called 30 s before a rise, Episode
// must return the episode that rise begins. Before #422 it returned the
// previous one for every moonrise at Barcelona, and for Sirius every time.
func TestEpisodeJustBeforeARiseIsThatRise(t *testing.T) {
	t.Parallel()

	prov := eph.Default()

	site, err := NewSiteEarthLocation("Barcelona", 41.39, 2.17, 0)
	if err != nil {
		t.Fatal(err)
	}

	sirius := NewStar("Sirius", angle.Deg(101.2872), angle.Deg(-16.7161))

	for _, c := range []struct {
		name       string
		target     Observable
		start, end time.Time
		want       int
	}{
		// 31 days of lunar days of 24h 50m.
		{"Moon", NewMoon(prov), time.Date(2026, 5, 1, 0, 0, 0, 0, time.LocationUTC), time.Date(2026, 6, 1, 0, 0, 0, 0, time.LocationUTC), 30},
		{"Sirius", sirius, time.Date(2026, 9, 1, 0, 0, 0, 0, time.LocationUTC), time.Date(2026, 9, 11, 0, 0, 0, 0, time.LocationUTC), 10},
	} {
		events, err := visibilityEvents(c.target, site, c.start, c.end)
		if err != nil {
			t.Fatal(err)
		}

		var rises []time.Time

		for _, e := range events {
			if e.Kind == EventRise {
				rises = append(rises, e.Time)
			}
		}

		if len(rises) != c.want {
			t.Fatalf("%s: %d rises, want %d", c.name, len(rises), c.want)
		}

		for _, at := range rises {
			rise, _, err := Episode(at.Add(unit.Seconds(-30)), at.Add(unit.Hours(1)), c.target, site)
			if err != nil {
				t.Fatal(err)
			}

			if rise == nil || rise.Time.Sub(at).Abs() > unit.Seconds(1) {
				t.Errorf("%s: Episode from 30 s before the rise at %v began at %v", c.name, at, rise)
			}
		}
	}
}
