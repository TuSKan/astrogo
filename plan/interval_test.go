package plan_test

import (
	"errors"
	"math"
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

// intervalSearches returns every function that searches an interval, each
// called over [start, end] and keeping only its error.
func intervalSearches(t *testing.T) map[string]func(start, end time.Time) error {
	t.Helper()

	prov := eph.Default()

	site, err := plan.NewSite("Barcelona", angle.LatDeg(41.39), angle.LonDeg(2.17), unit.Meters(0))
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

	return map[string]func(start, end time.Time) error{
		"EventSolver.Find": func(s, e time.Time) error {
			return errOf(plan.NewEventSolver(0, 0).Find(plan.EventSpec{
				Family: plan.EventFamilyVisibility, Kind: plan.EventRise, Target: sun, Observer: site,
			}, plan.Window{Start: s, End: e}))
		},
		"SunEvents": func(s, e time.Time) error { return errOf(plan.SunEvents(site, plan.Window{Start: s, End: e}, prov)) },
		"SunriseSunset": func(s, e time.Time) error {
			return errOf2(plan.SunriseSunset(site, plan.Window{Start: s, End: e}, prov))
		},
		"MoonEvents": func(s, e time.Time) error { return errOf(plan.MoonEvents(site, plan.Window{Start: s, End: e}, prov)) },
		"MoonriseMoonset": func(s, e time.Time) error {
			return errOf2(plan.MoonriseMoonset(site, plan.Window{Start: s, End: e}, prov))
		},
		"TwilightEvents": func(s, e time.Time) error {
			return errOf(plan.TwilightEvents(site, plan.Window{Start: s, End: e}, prov, plan.CivilTwilight))
		},
		"Conjunctions": func(s, e time.Time) error { return errOf(plan.Conjunctions(mars, sun, plan.Window{Start: s, End: e})) },
		"ConjunctionsEcliptic": func(s, e time.Time) error {
			return errOf(plan.ConjunctionsEcliptic(mars, sun, plan.Window{Start: s, End: e}))
		},
		"Appulses":    func(s, e time.Time) error { return errOf(plan.Appulses(moon, mars, plan.Window{Start: s, End: e})) },
		"Oppositions": func(s, e time.Time) error { return errOf(plan.Oppositions(mars, sun, plan.Window{Start: s, End: e})) },
		"GreatestElongations": func(s, e time.Time) error {
			return errOf(plan.GreatestElongations(mars, sun, plan.Window{Start: s, End: e}))
		},
		"FullMoonOppositions": func(s, e time.Time) error {
			return errOf(plan.FullMoonOppositions(plan.Window{Start: s, End: e}, prov))
		},
		"VisibilityEvents": func(s, e time.Time) error {
			return errOf(plan.VisibilityEvents(star, site, plan.Window{Start: s, End: e}))
		},
		"MoonPhases":    func(s, e time.Time) error { return errOf(plan.MoonPhases(plan.Window{Start: s, End: e}, prov)) },
		"LunarEclipses": func(s, e time.Time) error { return errOf(plan.LunarEclipses(plan.Window{Start: s, End: e}, prov)) },
		"SolarEclipses": func(s, e time.Time) error { return errOf(plan.SolarEclipses(plan.Window{Start: s, End: e}, prov)) },
		"RankObservable": func(s, e time.Time) error {
			return errOf(planner.RankObservable([]plan.Observable{star}, plan.Window{Start: s, End: e}))
		},
		"SatellitePasses": func(s, e time.Time) error {
			return errOf(plan.SatellitePasses(plan.NewSatellite("ISS", eph.Moon, prov), site, plan.Window{Start: s, End: e}, plan.WithMinElevation(angle.Deg(10))))
		},
		"TransitEstimate": func(s, e time.Time) error {
			return errOf2(plan.TransitEstimate(vega, site, plan.Window{Start: s, End: e}))
		},
		"MaxAltitudeInWindow": func(s, e time.Time) error {
			return errOf(plan.MaxAltitudeInWindow(vega, site, plan.Window{Start: s, End: e}))
		},
		"Episode": func(s, e time.Time) error { return errOf2(plan.Episode(star, site, plan.Window{Start: s, End: e})) },
		"Find": func(s, e time.Time) error {
			return errOf(plan.Find(vega, site, plan.Window{Start: s, End: e}, unit.Minutes(5)))
		},
		"VisibleIntervals": func(s, e time.Time) error {
			return errOf(plan.VisibleIntervals(vega, site, plan.Window{Start: s, End: e}, unit.Minutes(5), angle.Deg(10)))
		},
		"ObservableWindows": func(s, e time.Time) error {
			return errOf(plan.ObservableWindows(star, site, plan.Window{Start: s, End: e}, unit.Minutes(5)))
		},
	}
}

// TestEveryIntervalSearchRefusesAReversedInterval calls each function that
// searches an interval with its end before its start, by a minute and by two
// days, and requires ErrReversedInterval: never a panic, never an empty
// answer that could be taken for a quiet sky (#418). The minute is the case
// that indexed an empty sample list; the two days, the one that asked make
// for a negative capacity. VisibleIntervals and ObservableWindows were missing
// from the list, and had no check: both answered a reversed window with
// nothing and no error (#996).
func TestEveryIntervalSearchRefusesAReversedInterval(t *testing.T) {
	start := time.Date(2026, 9, 24, 12, 0, 0, 0, time.LocationUTC)

	for name, call := range intervalSearches(t) {
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

// TestEveryIntervalSearchRefusesANonFiniteInterval calls each function that
// searches an interval with a NaN or infinite start or end, and requires
// ErrNonFiniteInterval. Such a time compares false both ways, so it passed the
// reversed-interval check: 18 of these searches then panicked sizing their
// sample buffer, RankObservable panicked in a goroutine no caller could
// recover, Oppositions never returned, and the rest answered with no error
// (#996). FromJD(+Inf) is a case of its own, a time whose JD is NaN.
func TestEveryIntervalSearchRefusesANonFiniteInterval(t *testing.T) {
	start := time.Date(2026, 9, 24, 12, 0, 0, 0, time.LocationUTC)

	windows := map[string][2]time.Time{
		"end FromJD(+Inf)":   {start, time.FromJD(math.Inf(1), time.UTC)},
		"end FromJD(NaN)":    {start, time.FromJD(math.NaN(), time.UTC)},
		"start FromJD(NaN)":  {time.FromJD(math.NaN(), time.UTC), start},
		"start FromJD(-Inf)": {time.FromJD(math.Inf(-1), time.UTC), start},
	}

	for name, call := range intervalSearches(t) {
		for wname, w := range windows {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("%s with %s: panicked: %v", name, wname, r)
					}
				}()

				if err := call(w[0], w[1]); !errors.Is(err, plan.ErrNonFiniteInterval) {
					t.Errorf("%s with %s: err %v, want ErrNonFiniteInterval", name, wname, err)
				}
			}()
		}
	}
}

// TestAnEmptyIntervalIsNotReversed: start equal to end is a valid interval
// with nothing in it.
func TestAnEmptyIntervalIsNotReversed(t *testing.T) {
	at := time.Date(2026, 9, 24, 12, 0, 0, 0, time.LocationUTC)

	site, err := plan.NewSite("Barcelona", angle.LatDeg(41.39), angle.LonDeg(2.17), unit.Meters(0))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := plan.SunEvents(site, plan.Window{Start: at, End: at}, eph.Default()); err != nil {
		t.Errorf("SunEvents over an empty interval: %v", err)
	}

	if phases, err := plan.MoonPhases(plan.Window{Start: at, End: at}, eph.Default()); err != nil || len(phases) != 0 {
		t.Errorf("MoonPhases over an empty interval: %v, err %v", phases, err)
	}
}
