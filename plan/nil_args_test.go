package plan_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// errPanicked marks an error noPanic made from a panic.
var errPanicked = errors.New("panicked")

// noPanic runs call and turns a panic into an error wrapping errPanicked, so
// a table of calls reports every one that panics rather than stopping at the
// first.
func noPanic(call func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", errPanicked, r)
		}
	}()

	return call()
}

// TestConstraintsRefuseANilSiteOrTarget: every built-in constraint's Check
// refuses a nil site with ErrNilSite, and every one that reads its target
// refuses a nil one with ErrNilTarget, from Check and from CheckCtx. They
// used to panic on both (#1186). The constraints that read no target, the
// Sun's altitude, the Moon's illumination and a time window, answer for a nil
// one as for any other.
func TestConstraintsRefuseANilSiteOrTarget(t *testing.T) {
	t.Parallel()

	site, err := plan.NewSite("Paranal", angle.LatDeg(-24.6272), angle.LonDeg(-70.4045), unit.Meters(2635))
	if err != nil {
		t.Fatal(err)
	}

	at := time.Date(2026, time.June, 15, 4, 0, 0, 0, time.LocationUTC)
	ctx := mustContext(t, at, site.Location(), site.Refraction())

	for _, c := range []struct {
		constraint  plan.ConstraintCtx
		readsTarget bool
	}{
		{plan.Altitude{}, true},
		{plan.Airmass{}, true},
		{plan.Sun{}, false},
		{plan.MoonSep{}, true},
		{plan.MoonIllum{}, false},
		{plan.SunSep{}, true},
		{plan.GalacticLatitude{}, true},
		{plan.TimeWindow{}, false},
		{plan.Horizon{}, true},
		{plan.LimitingMagnitudeConstraint{}, true},
	} {
		name := fmt.Sprintf("%T", c.constraint)
		star := plan.NewStar("Vega", angle.Deg(279.23), angle.Deg(38.78))

		if err := noPanic(func() error { return errOf(c.constraint.Check(star, nil, at)) }); !errors.Is(err, plan.ErrNilSite) {
			t.Errorf("%s.Check with a nil site: %v, want ErrNilSite", name, err)
		}

		for _, via := range []struct {
			name  string
			check func() error
		}{
			{"Check", func() error { return errOf(c.constraint.Check(nil, site, at)) }},
			{"CheckCtx", func() error { return errOf(c.constraint.CheckCtx(nil, site, at, ctx)) }},
		} {
			err := noPanic(via.check)

			switch {
			case c.readsTarget && !errors.Is(err, plan.ErrNilTarget):
				t.Errorf("%s.%s with a nil target: %v, want ErrNilTarget", name, via.name, err)
			case !c.readsTarget && (errors.Is(err, plan.ErrNilTarget) || errors.Is(err, errPanicked)):
				t.Errorf("%s.%s with a nil target, which it does not read: %v", name, via.name, err)
			}
		}
	}
}

// TestSearchesRefuseANilSiteOrTarget: IsObservable, RankObservables and the
// interval searches that take a *Site refuse a nil one with ErrNilSite, and
// IsObservable a nil target with ErrNilTarget; SatellitePasses refuses a nil
// observer with coord.ErrNilGeodetic. Each used to panic (#1186).
func TestSearchesRefuseANilSiteOrTarget(t *testing.T) {
	t.Parallel()

	site, err := plan.NewSite("Paranal", angle.LatDeg(-24.6272), angle.LonDeg(-70.4045), unit.Meters(2635))
	if err != nil {
		t.Fatal(err)
	}

	start := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.LocationUTC)
	end := start.Add(unit.Hours(6))
	star := plan.NewStar("Vega", angle.Deg(279.23), angle.Deg(38.78))
	vega := fixedObject{coord.NewICRS(angle.Deg(279.23), angle.Deg(38.78))}

	for _, c := range []struct {
		name string
		call func() error
		want error
	}{
		{"IsObservable, nil target", func() error { return errOf(plan.IsObservable(nil, site, start)) }, plan.ErrNilTarget},
		{"IsObservable, nil site", func() error { return errOf(plan.IsObservable(star, nil, start)) }, plan.ErrNilSite},
		{"RankObservables", func() error {
			return errOf(plan.RankObservables([]plan.Observable{star}, nil, start))
		}, plan.ErrNilSite},
		{"IsVisible", func() error { return errOf(plan.IsVisible(vega, nil, start, angle.Deg(10))) }, plan.ErrNilSite},
		{"VisibleIntervals", func() error {
			return errOf(plan.VisibleIntervals(vega, nil, plan.Window{Start: start, End: end}, unit.Minutes(5), angle.Deg(10)))
		}, plan.ErrNilSite},
		{"TransitEstimate", func() error { return errOf2(plan.TransitEstimate(vega, nil, plan.Window{Start: start, End: end})) }, plan.ErrNilSite},
		{"Find", func() error { return errOf(plan.Find(vega, nil, plan.Window{Start: start, End: end}, unit.Minutes(5))) }, plan.ErrNilSite},
		{"ObservableWindows", func() error {
			return errOf(plan.ObservableWindows(star, nil, plan.Window{Start: start, End: end}, unit.Minutes(5)))
		}, plan.ErrNilSite},
		{"SatellitePasses", func() error {
			return errOf(plan.SatellitePasses(plan.NewSatellite("Moon", eph.Moon, eph.Default()), nil, plan.Window{Start: start, End: end}, plan.WithMinElevation(angle.Deg(10))))
		}, plan.ErrNilSite},
	} {
		if err := noPanic(c.call); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
}
