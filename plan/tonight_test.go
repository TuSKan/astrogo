package plan_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// near reports whether a and b are within the event solver's 1 s tolerance,
// with a second of slack for two searches that sampled differently.
func near(a, b time.Time) bool { return a.Sub(b).Abs() <= unit.Seconds(2) }

// TestTonightIsOneNightFromAnyInstantInIt: at Madrid, noon, 23:00, and 06:30
// the next morning, after astronomical dawn but before sunrise, all give one
// Tonight (#1194). The last is the instant NightOf alone would place in the
// next night, for astronomical twilight has ended; Tonight takes each
// twilight from the sunset that begins its night. Its events are in order,
// sunset through the three dusks to the three dawns and sunrise, and agree
// with SunEvents and NightOf.
func TestTonightIsOneNightFromAnyInstantInIt(t *testing.T) {
	t.Parallel()

	prov := eph.Default()

	site, err := plan.NewSite("Madrid", angle.LatDeg(40.4168), angle.LonDeg(-3.7038), unit.Meters(0))
	if err != nil {
		t.Fatal(err)
	}

	noon := time.Date(2026, time.November, 17, 12, 0, 0, 0, time.LocationUTC)

	want, err := plan.TonightAt(site, noon, prov)
	if err != nil {
		t.Fatalf("TonightAt noon: %v", err)
	}

	if want.Civil == nil || want.Nautical == nil || want.Astronomical == nil {
		t.Fatalf("a November night at Madrid has every twilight: %+v", want)
	}

	order := []struct {
		name string
		at   time.Time
	}{
		{"sunset", want.Sunset},
		{"civil dusk", want.Civil.Dusk},
		{"nautical dusk", want.Nautical.Dusk},
		{"astronomical dusk", want.Astronomical.Dusk},
		{"astronomical dawn", want.Astronomical.Dawn},
		{"nautical dawn", want.Nautical.Dawn},
		{"civil dawn", want.Civil.Dawn},
		{"sunrise", want.Sunrise},
	}

	for i := 1; i < len(order); i++ {
		if !order[i-1].at.Before(order[i].at) {
			t.Errorf("%s at %v is not before %s at %v", order[i-1].name, order[i-1].at, order[i].name, order[i].at)
		}
	}

	rise, set, err := plan.SunriseSunset(site, plan.Window{Start: noon, End: noon.Add(unit.Days(1))}, prov)
	if err != nil || rise == nil || set == nil {
		t.Fatalf("SunriseSunset: %v %v %v", rise, set, err)
	}

	if !near(want.Sunset, set.Time) || !near(want.Sunrise, rise.Time) {
		t.Errorf("Tonight %v to %v, SunEvents' sunset %v and sunrise %v", want.Sunset, want.Sunrise, set.Time, rise.Time)
	}

	astro, err := plan.NightOf(site, noon, prov, plan.AstronomicalTwilight)
	if err != nil || !near(astro.Dusk, want.Astronomical.Dusk) || !near(astro.Dawn, want.Astronomical.Dawn) {
		t.Errorf("Tonight's astronomical night %+v, NightOf's %+v (%v)", *want.Astronomical, astro, err)
	}

	for _, at := range []time.Time{
		noon.Add(unit.Hours(11)),                     // 23:00
		want.Astronomical.Dawn.Add(unit.Minutes(30)), // after astronomical dawn, before sunrise
	} {
		got, err := plan.TonightAt(site, at, prov)
		if err != nil {
			t.Fatalf("TonightAt %v: %v", at, err)
		}

		if !near(got.Sunset, want.Sunset) || !near(got.Sunrise, want.Sunrise) ||
			got.Astronomical == nil || !near(got.Astronomical.Dusk, want.Astronomical.Dusk) {
			t.Errorf("TonightAt %v = %v to %v, astronomical %+v; want the night from noon", at, got.Sunset, got.Sunrise, got.Astronomical)
		}
	}
}

// TestTonightLeavesOutTheTwilightTheSunDoesNotReach: in early July at London
// the Sun goes 15.5° down at most, so Tonight has a nautical night and no
// astronomical one, where the night it does have still runs sunset to
// sunrise. The last such night of the summer has none either, though the
// next has one. At Longyearbyen at the June solstice the Sun does not set, and
// TonightAt is ErrNoTwilight.
func TestTonightLeavesOutTheTwilightTheSunDoesNotReach(t *testing.T) {
	t.Parallel()

	prov := eph.Default()

	london, err := plan.NewSite("London", angle.LatDeg(51.5), angle.LonDeg(0), unit.Meters(0))
	if err != nil {
		t.Fatal(err)
	}

	tonight, err := plan.TonightAt(london, time.Date(2026, time.July, 2, 12, 0, 0, 0, time.LocationUTC), prov)
	if err != nil {
		t.Fatalf("TonightAt London: %v", err)
	}

	if tonight.Astronomical != nil || tonight.Nautical == nil || tonight.Civil == nil {
		t.Errorf("London in July: civil %v, nautical %v, astronomical %v; want the first two and no astronomical night",
			tonight.Civil, tonight.Nautical, tonight.Astronomical)
	}

	// The night astronomical twilight returns to London, late in July, and the
	// one before it, which has none: the next night's dusk is more than a day
	// after this night's sunset, so it is not taken for tonight's.
	var before, first plan.Tonight

	for day := 10; day <= 40; day++ {
		got, err := plan.TonightAt(london, time.Date(2026, time.July, day, 12, 0, 0, 0, time.LocationUTC), prov)
		if err != nil {
			t.Fatalf("TonightAt London, July %d: %v", day, err)
		}

		if got.Astronomical != nil {
			first = got

			break
		}

		before = got
	}

	if first.Astronomical == nil || before.Sunset.IsZero() {
		t.Fatalf("astronomical night does not return to London between 10 July and 9 August")
	}

	if !first.Astronomical.Dusk.After(before.Sunrise) || before.Nautical == nil {
		t.Errorf("the night before the first astronomical one, %v to %v, nautical %v; the first's astronomical dusk %v",
			before.Sunset, before.Sunrise, before.Nautical, first.Astronomical.Dusk)
	}

	longyearbyen, err := plan.NewSite("Longyearbyen", angle.LatDeg(78.2232), angle.LonDeg(15.6267), unit.Meters(0))
	if err != nil {
		t.Fatal(err)
	}

	if got, err := plan.TonightAt(longyearbyen, time.Date(2026, time.June, 21, 11, 0, 0, 0, time.LocationUTC), prov); !errors.Is(err, plan.ErrNoTwilight) {
		t.Errorf("TonightAt under the midnight Sun = %+v, %v, want ErrNoTwilight", got, err)
	}
}

// TestTonightPlacesTheMoon: the Moon's rise and set inside the night are
// MoonEvents', whether it is up at sunset agrees with the order of those
// events, and its illumination is MoonIllumination's at the night's middle.
// Two nights two weeks apart cover both cases: on 2026-11-17 a Moon past
// first quarter is up at sunset and sets around midnight, and on 2026-12-01
// a waning one is down at sunset and rises after midnight.
func TestTonightPlacesTheMoon(t *testing.T) {
	t.Parallel()

	prov := eph.Default()

	site, err := plan.NewSite("Madrid", angle.LatDeg(40.4168), angle.LonDeg(-3.7038), unit.Meters(0))
	if err != nil {
		t.Fatal(err)
	}

	for _, noon := range []time.Time{
		time.Date(2026, time.November, 17, 12, 0, 0, 0, time.LocationUTC),
		time.Date(2026, time.December, 1, 12, 0, 0, 0, time.LocationUTC),
	} {
		tonight, err := plan.TonightAt(site, noon, prov)
		if err != nil {
			t.Fatalf("TonightAt %v: %v", noon, err)
		}

		rise, set, err := plan.MoonriseMoonset(site, plan.Window{Start: tonight.Sunset, End: tonight.Sunrise}, prov)
		if err != nil {
			t.Fatal(err)
		}

		for _, c := range []struct {
			name      string
			got, want *time.Time
		}{
			{"moonrise", tonight.Moonrise, eventTime(rise)},
			{"moonset", tonight.Moonset, eventTime(set)},
		} {
			if (c.got == nil) != (c.want == nil) || c.got != nil && !near(*c.got, *c.want) {
				t.Errorf("%v: %s %v, MoonEvents' %v", noon, c.name, c.got, c.want)
			}
		}

		// A set before any rise means it was up at sunset; a rise before any
		// set means it was down.
		switch {
		case tonight.Moonset != nil && (tonight.Moonrise == nil || tonight.Moonset.Before(*tonight.Moonrise)):
			if !tonight.MoonUpAtSunset {
				t.Errorf("%v: the Moon sets at %v before rising, but MoonUpAtSunset is false", noon, *tonight.Moonset)
			}
		case tonight.Moonrise != nil:
			if tonight.MoonUpAtSunset {
				t.Errorf("%v: the Moon rises at %v before setting, but MoonUpAtSunset is true", noon, *tonight.Moonrise)
			}
		}

		middle := tonight.Sunset.Add(tonight.Sunrise.Sub(tonight.Sunset) / 2)

		frac, _, err := plan.MoonIllumination(middle, prov)
		if err != nil {
			t.Fatal(err)
		}

		if math.Abs(tonight.MoonIllumination-frac) > 1e-12 {
			t.Errorf("%v: MoonIllumination %v, MoonIllumination at the middle %v", noon, tonight.MoonIllumination, frac)
		}
	}
}

// eventTime is e's time, or nil for no event.
func eventTime(e *plan.Event) *time.Time {
	if e == nil {
		return nil
	}

	return &e.Time
}
