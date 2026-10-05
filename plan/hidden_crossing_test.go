package plan

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestCrossingPairsBetweenSamples are #426's three cases: a body whose daily
// extreme only just passes the threshold, so that it crosses and crosses back
// within one 15-minute step and no sample lands on the far side. The helpers
// returned nothing for all three. The instants are Skyfield 1.54's, for the
// same definition — the geometric topocentric centre, no refraction (DE421).
//
// The tolerances are wide for an event time because these crossings are
// shallow: the Sun at Mawson climbs 4.5e-5 °/s through its threshold, so 1″
// of altitude is 6 s of time, and astrogo and Skyfield differ here by about
// that. The sweep's own answer does not depend on its step — a 20-second
// sweep gives the same instants to 0.1 s — which is what
// TestTheStepDoesNotDecideWhichCrossingsExist holds it to.
func TestCrossingPairsBetweenSamples(t *testing.T) {
	prov := eph.Default()

	type pair struct {
		name        string
		first, last *Event
		want        [2]time.Time
		tol         unit.Duration
	}

	site := func(name string, lat, lon float64) *Site {
		s, err := NewSiteEarthLocation(name, lat, lon, 0)
		if err != nil {
			t.Fatal(err)
		}

		return s
	}

	cases := make([]pair, 0, 3)

	// Mawson in midwinter: the Sun is up for 14 minutes, a hundredth of a
	// degree above the rise/set threshold at its highest.
	start := time.Date(2026, 6, 28, 19, 0, 0, 0, time.LocationUTC)

	rise, set, err := SunriseSunset(start, start.Add(unit.Days(1)), site("Mawson", -67.6027, 62.8738), prov)
	if err != nil {
		t.Fatal(err)
	}

	cases = append(cases, pair{"Mawson sunrise and sunset", rise, set, [2]time.Time{
		time.Date(2026, 6, 29, 7, 45, 6, 0, time.LocationUTC),
		time.Date(2026, 6, 29, 7, 59, 6, 0, time.LocationUTC),
	}, unit.Seconds(12)})

	// Paris in June: ten minutes of astronomical night, 0.009° deep.
	start = time.Date(2026, 6, 11, 10, 0, 0, 0, time.LocationUTC)

	dawn, dusk, err := AstronomicalDawnDusk(start, start.Add(unit.Days(1)), site("Paris", 48.8566, 2.3522), prov)
	if err != nil {
		t.Fatal(err)
	}

	cases = append(cases, pair{"Paris astronomical dusk and dawn", dusk, dawn, [2]time.Time{
		time.Date(2026, 6, 11, 23, 45, 20, 0, time.LocationUTC),
		time.Date(2026, 6, 11, 23, 55, 18, 0, time.LocationUTC),
	}, unit.Seconds(12)})

	// Alta: the Moon sets for twelve minutes, 0.006° below the threshold.
	start = time.Date(2026, 6, 17, 22, 0, 0, 0, time.LocationUTC)

	mrise, mset, err := MoonriseMoonset(start, start.Add(unit.Days(1)), site("Alta", 69.9689, 23.2716), prov)
	if err != nil {
		t.Fatal(err)
	}

	cases = append(cases, pair{"Alta moonset and moonrise", mset, mrise, [2]time.Time{
		time.Date(2026, 6, 18, 1, 30, 45, 0, time.LocationUTC),
		time.Date(2026, 6, 18, 1, 42, 28, 0, time.LocationUTC),
	}, unit.Seconds(20)})

	for _, c := range cases {
		if c.first == nil || c.last == nil {
			t.Errorf("%s: got %v and %v, want both", c.name, c.first, c.last)

			continue
		}

		for k, got := range []time.Time{c.first.Time, c.last.Time} {
			if d := got.Sub(c.want[k]).Abs(); d > c.tol {
				t.Errorf("%s: %v, Skyfield gives %v (off by %v)", c.name, got, c.want[k], d)
			}
		}
	}
}

// TestTheStepDoesNotDecideWhichCrossingsExist: a 15-minute sweep must find
// every rise and set a 2-minute sweep finds, over the days where the Sun's
// daily extreme crosses the threshold — around June 12 and July 1, when its
// declination is 23.2°, the Sun at Mawson stops and starts clearing the
// horizon at midday, and astronomical night at Paris ends and returns. Before
// #426 the 15-minute sweep dropped the pairs that fit between two samples.
func TestTheStepDoesNotDecideWhichCrossingsExist(t *testing.T) {
	t.Parallel()

	prov := eph.Default()

	for _, c := range []struct {
		name       string
		lat, lon   float64
		threshold  angle.Angle
		start, end time.Time
	}{
		{"Mawson in June, sunrise and sunset", -67.6027, 62.8738, angle.Deg(-0.8334),
			time.Date(2026, 6, 8, 0, 0, 0, 0, time.LocationUTC), time.Date(2026, 6, 16, 0, 0, 0, 0, time.LocationUTC)},
		{"Mawson in July, sunrise and sunset", -67.6027, 62.8738, angle.Deg(-0.8334),
			time.Date(2026, 6, 26, 0, 0, 0, 0, time.LocationUTC), time.Date(2026, 7, 4, 0, 0, 0, 0, time.LocationUTC)},
		{"Paris in June, astronomical twilight", 48.8566, 2.3522, angle.Deg(-18),
			time.Date(2026, 6, 5, 0, 0, 0, 0, time.LocationUTC), time.Date(2026, 6, 13, 0, 0, 0, 0, time.LocationUTC)},
		{"Paris in July, astronomical twilight", 48.8566, 2.3522, angle.Deg(-18),
			time.Date(2026, 6, 28, 0, 0, 0, 0, time.LocationUTC), time.Date(2026, 7, 6, 0, 0, 0, 0, time.LocationUTC)},
	} {
		site, err := NewSiteEarthLocation(c.name, c.lat, c.lon, 0)
		if err != nil {
			t.Fatal(err)
		}

		spec := EventSpec{
			Family: EventFamilyVisibility, Kind: EventAnyVisibility,
			Target: NewSun(prov), Observer: site, Threshold: c.threshold,
		}

		var runs [2][]Event

		for k, step := range []unit.Duration{unit.Minutes(2), unit.Minutes(15)} {
			events, err := NewEventSolver(step, unit.Seconds(1)).Find(spec, c.start, c.end)
			if err != nil {
				t.Fatal(err)
			}

			for _, e := range events {
				if e.Kind == EventRise || e.Kind == EventSet {
					runs[k] = append(runs[k], e)
				}
			}
		}

		fine, coarse := runs[0], runs[1]
		if len(coarse) != len(fine) {
			t.Errorf("%s: %d crossings at a 15-minute step, %d at a 2-minute step", c.name, len(coarse), len(fine))

			continue
		}

		for k := range fine {
			if fine[k].Kind != coarse[k].Kind || fine[k].Time.Sub(coarse[k].Time).Abs() > unit.Seconds(2) {
				t.Errorf("%s: crossing %d is %v at a 2-minute step, %v at 15", c.name, k, fine[k], coarse[k])
			}
		}
	}
}
