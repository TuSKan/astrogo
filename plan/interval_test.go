package plan_test

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// errOf and errOf2 keep only a call's error.
func errOf[T any](_ T, err error) error { return err }

func errOf2[A, B any](_ A, _ B, err error) error { return err }

// fixedObject is a coord.Object at a fixed ICRS position.
type fixedObject struct{ pos coord.ICRS }

func (o fixedObject) ICRS(time.Time) (coord.ICRS, error) { return o.pos, nil }

// TestEveryIntervalSearchRefusesAReversedInterval calls each function that
// searches an interval with its end before its start, by a minute and by two
// days, and requires ErrReversedInterval: never a panic, never an empty
// answer that could be taken for a quiet sky (#418). The minute is the case
// that indexed an empty sample list; the two days, the one that asked make
// for a negative capacity.
func TestEveryIntervalSearchRefusesAReversedInterval(t *testing.T) {
	prov := eph.Default()

	site, err := plan.NewSiteEarthLocation("Barcelona", 41.39, 2.17, 0)
	if err != nil {
		t.Fatal(err)
	}

	sun, moon, mars := plan.NewSun(prov), plan.NewMoon(prov), plan.NewMars(prov)
	vega := fixedObject{coord.NewICRS(angle.Deg(279.23), angle.Deg(38.78))}
	star := plan.NewStar("Vega", angle.Deg(279.23), angle.Deg(38.78))

	planner, err := plan.NewPlanner(site, nil)
	if err != nil {
		t.Fatal(err)
	}

	calls := map[string]func(start, end time.Time) error{
		"EventSolver.Find": func(s, e time.Time) error {
			return errOf(plan.NewEventSolver(0, 0).Find(plan.EventSpec{
				Family: plan.EventFamilyVisibility, Kind: plan.EventRise, Target: sun, Observer: site,
			}, s, e))
		},
		"SunEvents":       func(s, e time.Time) error { return errOf(plan.SunEvents(s, e, site, prov)) },
		"SunriseSunset":   func(s, e time.Time) error { return errOf2(plan.SunriseSunset(s, e, site, prov)) },
		"MoonEvents":      func(s, e time.Time) error { return errOf(plan.MoonEvents(s, e, site, prov)) },
		"MoonriseMoonset": func(s, e time.Time) error { return errOf2(plan.MoonriseMoonset(s, e, site, prov)) },
		"TwilightEvents": func(s, e time.Time) error {
			return errOf(plan.TwilightEvents(s, e, site, prov, plan.CivilTwilight))
		},
		"CivilDawnDusk":        func(s, e time.Time) error { return errOf2(plan.CivilDawnDusk(s, e, site, prov)) },
		"NauticalDawnDusk":     func(s, e time.Time) error { return errOf2(plan.NauticalDawnDusk(s, e, site, prov)) },
		"AstronomicalDawnDusk": func(s, e time.Time) error { return errOf2(plan.AstronomicalDawnDusk(s, e, site, prov)) },
		"Conjunctions":         func(s, e time.Time) error { return errOf(plan.Conjunctions(s, e, mars, sun)) },
		"ConjunctionsEcliptic": func(s, e time.Time) error { return errOf(plan.ConjunctionsEcliptic(s, e, mars, sun)) },
		"Appulses":             func(s, e time.Time) error { return errOf(plan.Appulses(s, e, moon, mars)) },
		"Oppositions":          func(s, e time.Time) error { return errOf(plan.Oppositions(s, e, mars, sun)) },
		"GreatestElongations":  func(s, e time.Time) error { return errOf(plan.GreatestElongations(s, e, mars, sun)) },
		"FullMoonOppositions":  func(s, e time.Time) error { return errOf(plan.FullMoonOppositions(s, e, prov)) },
		"VisibilityEvents":     func(s, e time.Time) error { return errOf(plan.VisibilityEvents(s, e, star, site)) },
		"MoonPhases":           func(s, e time.Time) error { return errOf(plan.MoonPhases(s, e, prov)) },
		"LunarEclipses":        func(s, e time.Time) error { return errOf(plan.LunarEclipses(s, e, prov)) },
		"SolarEclipses":        func(s, e time.Time) error { return errOf(plan.SolarEclipses(s, e, prov)) },
		"RankObservable": func(s, e time.Time) error {
			return errOf(planner.RankObservable([]plan.Observable{star}, s, e))
		},
		"SatellitePasses": func(s, e time.Time) error {
			return errOf(plan.SatellitePasses(prov, "ISS", s, e, site.Location(), angle.Deg(10)))
		},
		"TransitEstimate":     func(s, e time.Time) error { return errOf2(plan.TransitEstimate(vega, site, s, e)) },
		"MaxAltitudeInWindow": func(s, e time.Time) error { return errOf(plan.MaxAltitudeInWindow(vega, site, s, e)) },
		"Episode":             func(s, e time.Time) error { return errOf2(plan.Episode(s, e, star, site)) },
		"Find":                func(s, e time.Time) error { return errOf(plan.Find(vega, site, nil, s, e, 5*time.Minute)) },
	}

	start := time.Date(2026, 9, 24, 12, 0, 0, 0, time.LocationUTC)

	for name, call := range calls {
		for _, back := range []unit.Duration{unit.Minutes(1), unit.Days(2)} {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("%s over an interval reversed by %v: panicked: %v", name, back, r)
					}
				}()

				if err := call(start, start.Add(-back)); !errors.Is(err, plan.ErrReversedInterval) {
					t.Errorf("%s over an interval reversed by %v: err %v, want ErrReversedInterval", name, back, err)
				}
			}()
		}
	}
}

// TestAnEmptyIntervalIsNotReversed: start equal to end is a valid interval
// with nothing in it.
func TestAnEmptyIntervalIsNotReversed(t *testing.T) {
	at := time.Date(2026, 9, 24, 12, 0, 0, 0, time.LocationUTC)

	site, err := plan.NewSiteEarthLocation("Barcelona", 41.39, 2.17, 0)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := plan.SunEvents(at, at, site, eph.Default()); err != nil {
		t.Errorf("SunEvents over an empty interval: %v", err)
	}

	if phases, err := plan.MoonPhases(at, at, eph.Default()); err != nil || len(phases) != 0 {
		t.Errorf("MoonPhases over an empty interval: %v, err %v", phases, err)
	}
}
