package plan_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/angle"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/unit"
)

// TestNightOfIsOneNightFromAnyInstantInIt: from noon, from late evening and
// from the small hours, NightOf returns the same night, dusk before dawn and
// less than a day apart. The dawn/dusk helpers it replaces returned, when
// called after dusk, that morning's dawn and the next evening's dusk (#1190).
// Each night is the dusk and dawn TwilightEvents pairs over a window holding
// it, found from different sweep samples, so they agree to the solver's 1 s
// tolerance rather than exactly.
func TestNightOfIsOneNightFromAnyInstantInIt(t *testing.T) {
	t.Parallel()

	prov := eph.Default()

	site, err := plan.NewSite("Madrid", angle.LatDeg(40.4168), angle.LonDeg(-3.7038), unit.Meters(0))
	if err != nil {
		t.Fatal(err)
	}

	noon := epoch.Date(2026, time.November, 17, 12, 0, 0, 0, time.UTC)

	for _, kind := range []plan.TwilightKind{plan.CivilTwilight, plan.NauticalTwilight, plan.AstronomicalTwilight} {
		pairs, err := plan.TwilightEvents(site, plan.Window{Start: noon, End: noon.Add(unit.Hours(24))}, prov, kind)
		if err != nil {
			t.Fatalf("%s: TwilightEvents: %v", kind, err)
		}

		if len(pairs) != 1 || pairs[0].Dusk == nil || pairs[0].Dawn == nil {
			t.Fatalf("%s: TwilightEvents gives %v, want one night", kind, pairs)
		}

		want := plan.Night{Dusk: pairs[0].Dusk.Time, Dawn: pairs[0].Dawn.Time, Kind: kind}

		for _, at := range []struct {
			name string
			t    epoch.Time
		}{
			{"noon", noon},
			{"23:00", noon.Add(unit.Hours(11))},
			{"03:00 the next day", noon.Add(unit.Hours(15))},
		} {
			got, err := plan.NightOf(site, at.t, prov, kind)
			if err != nil {
				t.Fatalf("%s from %s: %v", kind, at.name, err)
			}

			if got.Kind != kind ||
				got.Dusk.Sub(want.Dusk).Abs() > unit.Seconds(1) || got.Dawn.Sub(want.Dawn).Abs() > unit.Seconds(1) {
				t.Errorf("%s from %s: %v to %v (%s), want %v to %v", kind, at.name,
					got.Dusk, got.Dawn, got.Kind, want.Dusk, want.Dawn)
			}

			if !got.Dusk.Before(got.Dawn) || got.Dawn.Sub(got.Dusk) >= unit.Days(1) {
				t.Errorf("%s from %s: dusk %v and dawn %v are not one night", kind, at.name, got.Dusk, got.Dawn)
			}
		}
	}
}

// TestNightOfAfterDawnIsTheComingNight: an instant just after a night's dawn
// is in daylight, so NightOf returns the night that begins that evening, even
// when that night's dawn is more than a day away, and an instant just before
// the dawn is still in the night it ends.
func TestNightOfAfterDawnIsTheComingNight(t *testing.T) {
	t.Parallel()

	prov := eph.Default()

	site, err := plan.NewSite("Madrid", angle.LatDeg(40.4168), angle.LonDeg(-3.7038), unit.Meters(0))
	if err != nil {
		t.Fatal(err)
	}

	tonight, err := plan.NightOf(site, epoch.Date(2026, time.November, 17, 23, 0, 0, 0, time.UTC), prov, plan.AstronomicalTwilight)
	if err != nil {
		t.Fatal(err)
	}

	before, err := plan.NightOf(site, tonight.Dawn.Add(unit.Minutes(-1)), prov, plan.AstronomicalTwilight)
	if err != nil {
		t.Fatal(err)
	}

	if before.Dusk.Sub(tonight.Dusk).Abs() > unit.Seconds(1) {
		t.Errorf("a minute before dawn: night from %v, want the night from %v", before.Dusk, tonight.Dusk)
	}

	// A second after dawn. In November each dawn comes about a minute later
	// than the one before, so the coming night ends more than a day after
	// this instant, past the first day of crossings NightOf looks through.
	justAfter := tonight.Dawn.Add(unit.Seconds(1))

	after, err := plan.NightOf(site, justAfter, prov, plan.AstronomicalTwilight)
	if err != nil {
		t.Fatal(err)
	}

	if gap := after.Dusk.Sub(tonight.Dawn); gap < unit.Hours(6) || gap > unit.Hours(18) {
		t.Errorf("a second after dawn at %v: night from %v, want the next evening's", tonight.Dawn, after.Dusk)
	}

	if after.Dawn.Sub(justAfter) <= unit.Days(1) {
		t.Errorf("the next dawn, %v, is within a day of %v: the case no longer reaches past the first day", after.Dawn, justAfter)
	}
}

// TestNightOfFindsNoNightThatLastsADayOrNever: at Longyearbyen, 78.2°N, the
// Sun does not set at the June solstice, 11.6° up at its lowest, and does not
// climb past 11.6° down at the December one, so June has no astronomical
// night and December's civil night lasts weeks. Both are ErrNoTwilight, from
// day and from night.
func TestNightOfFindsNoNightThatLastsADayOrNever(t *testing.T) {
	t.Parallel()

	prov := eph.Default()

	site, err := plan.NewSite("Longyearbyen", angle.LatDeg(78.2232), angle.LonDeg(15.6267), unit.Meters(0))
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name string
		at   epoch.Time
		kind plan.TwilightKind
	}{
		{"June, astronomical, noon", epoch.Date(2026, time.June, 21, 11, 0, 0, 0, time.UTC), plan.AstronomicalTwilight},
		{"June, astronomical, midnight", epoch.Date(2026, time.June, 21, 23, 0, 0, 0, time.UTC), plan.AstronomicalTwilight},
		{"December, civil, noon", epoch.Date(2026, time.December, 21, 11, 0, 0, 0, time.UTC), plan.CivilTwilight},
		{"December, civil, midnight", epoch.Date(2026, time.December, 21, 23, 0, 0, 0, time.UTC), plan.CivilTwilight},
	} {
		if n, err := plan.NightOf(site, c.at, prov, c.kind); !errors.Is(err, plan.ErrNoTwilight) {
			t.Errorf("%s: %+v, %v, want ErrNoTwilight", c.name, n, err)
		}
	}
}

// TestNightOfRefusesWhatIsNotAnInstantOrAKind: a kind TwilightThreshold does
// not know is ErrUnknownTwilightKind, never a night at some guessed
// threshold, and a time that is not finite is ErrNonFiniteInterval, as every
// interval search in this package refuses it.
func TestNightOfRefusesWhatIsNotAnInstantOrAKind(t *testing.T) {
	t.Parallel()

	prov := eph.Default()

	site, err := plan.NewSite("Madrid", angle.LatDeg(40.4168), angle.LonDeg(-3.7038), unit.Meters(0))
	if err != nil {
		t.Fatal(err)
	}

	noon := epoch.Date(2026, time.November, 17, 12, 0, 0, 0, time.UTC)

	if _, err := plan.NightOf(site, noon, prov, plan.TwilightKind(99)); !errors.Is(err, plan.ErrUnknownTwilightKind) {
		t.Errorf("kind 99: %v, want ErrUnknownTwilightKind", err)
	}

	for _, jd := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := plan.NightOf(site, epoch.FromJD(jd, epoch.UTC), prov, plan.CivilTwilight); !errors.Is(err, plan.ErrNonFiniteInterval) {
			t.Errorf("FromJD(%v): %v, want ErrNonFiniteInterval", jd, err)
		}
	}
}
