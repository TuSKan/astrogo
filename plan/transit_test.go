package plan

import (
	"math"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// transitsOf keeps the transits among events.
func transitsOf(events []Event) []time.Time {
	var out []time.Time

	for _, e := range events {
		if e.Kind == EventTransit {
			out = append(out, e.Time)
		}
	}

	return out
}

// requireTransits fails unless got holds exactly the want instants, each
// within tol.
func requireTransits(t *testing.T, what string, got, want []time.Time, tol unit.Duration) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("%s: %d transits %v, want %d %v", what, len(got), got, len(want), want)
	}

	for n := range want {
		if d := got[n].Sub(want[n]).Abs(); d > tol {
			t.Errorf("%s: transit %v, want %v (off by %v)", what, got[n], want[n], d)
		}
	}
}

// TestTransitsWhereTheAltitudeMaximumLeavesTheMeridian: at 75°N the Moon's
// declination changes fast enough against its diurnal swing that its highest
// point comes up to 15 minutes before the meridian, and at 89°N so does the
// Sun's. The solver looked for a transit only within a step of the sampled
// altitude maximum, and reported one of these three lunar transits and none of
// the solar (#417). The instants are Skyfield 1.54's
// almanac.meridian_transits (DE421), to the second; all four come within
// 0.53 s of them.
func TestTransitsWhereTheAltitudeMaximumLeavesTheMeridian(t *testing.T) {
	prov := eph.Default()

	north75, err := NewSiteEarthLocation("75N", 75, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	moon, err := MoonEvents(time.Date(2026, 1, 8, 0, 0, 0, 0, time.LocationUTC),
		time.Date(2026, 1, 11, 0, 0, 0, 0, time.LocationUTC), north75, prov)
	if err != nil {
		t.Fatal(err)
	}

	requireTransits(t, "Moon at 75°N", transitsOf(moon), []time.Time{
		time.Date(2026, 1, 8, 4, 8, 50, 0, time.LocationUTC),
		time.Date(2026, 1, 9, 4, 51, 3, 0, time.LocationUTC),
		time.Date(2026, 1, 10, 5, 32, 22, 0, time.LocationUTC),
	}, unit.Seconds(2))

	north89, err := NewSiteEarthLocation("89N", 89, 15, 0)
	if err != nil {
		t.Fatal(err)
	}

	day := time.Date(2026, 1, 28, 0, 0, 0, 0, time.LocationUTC)

	sun, err := SunEvents(day, day.Add(unit.Days(1)), north89, prov)
	if err != nil {
		t.Fatal(err)
	}

	requireTransits(t, "Sun at 89°N", transitsOf(sun), []time.Time{
		time.Date(2026, 1, 28, 11, 12, 54, 0, time.LocationUTC),
	}, unit.Seconds(2))
}

// TestTransitInTheWindowsFirstStep: a transit within the first step after
// start was never reported, since the altitude test needed a sample before the
// peak (#417). The instants are the issue's, to the second.
func TestTransitInTheWindowsFirstStep(t *testing.T) {
	prov := eph.Default()

	brno, err := NewSiteEarthLocation("Brno", 49.195, 16.608, 0)
	if err != nil {
		t.Fatal(err)
	}

	// The Sun transits at 10:45:36, three minutes in.
	start := time.Date(2026, 9, 24, 10, 42, 36, 0, time.LocationUTC)

	sun, err := SunEvents(start, start.Add(unit.Hours(12)), brno, prov)
	if err != nil {
		t.Fatal(err)
	}

	requireTransits(t, "Sun at Brno", transitsOf(sun), []time.Time{
		time.Date(2026, 9, 24, 10, 45, 36, 0, time.LocationUTC),
	}, unit.Seconds(2))

	// The Moon transits at 22:04:25, four minutes in, and not again within
	// the day.
	start = time.Date(2026, 3, 31, 22, 0, 0, 0, time.LocationUTC)

	moon, err := MoonEvents(start, start.Add(unit.Hours(24)), brno, prov)
	if err != nil {
		t.Fatal(err)
	}

	requireTransits(t, "Moon at Brno", transitsOf(moon), []time.Time{
		time.Date(2026, 3, 31, 22, 4, 25, 0, time.LocationUTC),
	}, unit.Seconds(2))
}

// TestTransitsDoNotDependOnTheStep: the hour-angle root was searched only
// within one step of the sampled altitude peak, so a finer step, which should
// only ever find more, found fewer — at 69°N a 15-minute step found both lunar
// transits of July 20–22, 2026, and a 1-minute step found none (#417).
func TestTransitsDoNotDependOnTheStep(t *testing.T) {
	prov := eph.Default()

	site, err := NewSiteEarthLocation("69N", 69, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	start := time.Date(2026, 7, 20, 0, 0, 0, 0, time.LocationUTC)
	end := time.Date(2026, 7, 22, 0, 0, 0, 0, time.LocationUTC)
	spec := EventSpec{Family: EventFamilyVisibility, Kind: EventTransit, Target: NewMoon(prov), Observer: site}

	steps := []unit.Duration{unit.Minutes(1), unit.Minutes(15), unit.Hours(2)}
	runs := make([][]time.Time, 0, len(steps))

	for _, step := range steps {
		events, err := NewEventSolver(step, unit.Seconds(1)).Find(spec, start, end)
		if err != nil {
			t.Fatal(err)
		}

		runs = append(runs, transitsOf(events))
	}

	if len(runs[0]) != 2 {
		t.Fatalf("1-minute step: %d transits %v, want 2", len(runs[0]), runs[0])
	}

	for _, run := range runs[1:] {
		requireTransits(t, "coarser step", run, runs[0], unit.Seconds(1))
	}
}

// TestEveryLunarMeridianPassageIsATransit: the Moon crosses the meridian
// about 352 times a year at every latitude. At 85°N the solver reported 61 of
// 2026's (#417). Over the first quarter of 2026 it must report all of them,
// one per lunar day, 24h 50m apart on average.
func TestEveryLunarMeridianPassageIsATransit(t *testing.T) {
	prov := eph.Default()

	site, err := NewSiteEarthLocation("85N", 85, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)
	end := time.Date(2026, 4, 1, 0, 0, 0, 0, time.LocationUTC)

	events, err := MoonEvents(start, end, site, prov)
	if err != nil {
		t.Fatal(err)
	}

	transits := transitsOf(events)

	// 90 days of lunar days of 24h 50.47m is 86.99; the first comes on
	// January 1 and the last on March 31.
	if len(transits) != 87 {
		t.Errorf("Moon at 85°N, first quarter of 2026: %d transits, want 87", len(transits))
	}

	for n := 1; n < len(transits); n++ {
		if gap := transits[n].Sub(transits[n-1]); gap < unit.Hours(24) || gap > unit.Hours(26) {
			t.Errorf("transits %v and %v are %v apart, want a lunar day", transits[n-1], transits[n], gap)
		}
	}
}

// TestTransitIsWhereSiderealTimeMeetsRightAscension checks transits against
// the definition, by a path that shares nothing with the solver's: the local
// apparent sidereal time equals the body's apparent right ascension, from
// eph.ApparentState rotated to the true equator and equinox of date. Diurnal
// parallax does not move the instant, so the geocentric place serves for the
// Moon too. They agree to 0.2″. Until #417 the transit search passed the
// apparent place to a routine that applies aberration itself, which put the
// Sun's transits some 19″ of hour angle, 1.3 s, early, and the Moon's up to
// 19″ late.
func TestTransitIsWhereSiderealTimeMeetsRightAscension(t *testing.T) {
	prov := eph.Default()
	start := time.Date(2026, 3, 31, 0, 0, 0, 0, time.LocationUTC)

	// A millisecond, where SunEvents and MoonEvents settle for a second.
	solver := NewEventSolver(unit.Minutes(15), unit.Seconds(0.001))

	for _, s := range []struct{ lat, lon float64 }{{49.195, 16.608}, {75, 0}} {
		site, err := NewSiteEarthLocation("site", s.lat, s.lon, 0)
		if err != nil {
			t.Fatal(err)
		}

		for _, id := range []eph.ID{eph.Sun, eph.Moon} {
			events, err := solver.Find(EventSpec{
				Family: EventFamilyVisibility, Kind: EventTransit, Target: NewPlanet(id.String(), id, prov), Observer: site,
			}, start, start.Add(unit.Days(3)))
			if err != nil {
				t.Fatal(err)
			}

			transits := transitsOf(events)
			if len(transits) < 2 {
				t.Fatalf("%v at %v°N: %d transits in three days", id, s.lat, len(transits))
			}

			for _, at := range transits {
				st, err := eph.ApparentState(prov, id, at)
				if err != nil {
					t.Fatal(err)
				}

				gast, err := at.GAST()
				if err != nil {
					t.Fatal(err)
				}

				tt1, tt2 := at.TT().JDParts()
				npb := gofaext.Pnm06a(tt1, tt2)
				x := npb[0][0]*st.Pos.X + npb[0][1]*st.Pos.Y + npb[0][2]*st.Pos.Z
				y := npb[1][0]*st.Pos.X + npb[1][1]*st.Pos.Y + npb[1][2]*st.Pos.Z

				ha := math.Remainder(gast.Degrees()+s.lon-math.Atan2(y, x)*180/math.Pi, 360) * 3600
				if math.Abs(ha) > 1 {
					t.Errorf("%v at %v°N, transit %v: hour angle %.2f″, want 0 within the 0.3″ of diurnal aberration",
						id, s.lat, at, ha)
				}
			}
		}
	}
}
