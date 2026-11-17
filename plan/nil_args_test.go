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

	site, err := plan.NewSiteEarthLocation("Paranal", -24.6272, -70.4045, 2635)
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

		if err := noPanic(func() error { _, err := c.constraint.Check(star, at, nil); return err }); !errors.Is(err, plan.ErrNilSite) {
			t.Errorf("%s.Check with a nil site: %v, want ErrNilSite", name, err)
		}

		for _, via := range []struct {
			name  string
			check func() error
		}{
			{"Check", func() error { _, err := c.constraint.Check(nil, at, site); return err }},
			{"CheckCtx", func() error { _, err := c.constraint.CheckCtx(nil, at, site, ctx); return err }},
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

	site, err := plan.NewSiteEarthLocation("Paranal", -24.6272, -70.4045, 2635)
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
		{"IsObservable, nil target", func() error { _, err := plan.IsObservable(nil, start, site); return err }, plan.ErrNilTarget},
		{"IsObservable, nil site", func() error { _, err := plan.IsObservable(star, start, nil); return err }, plan.ErrNilSite},
		{"RankObservables", func() error {
			_, err := plan.RankObservables([]plan.Observable{star}, start, nil)
			return err
		}, plan.ErrNilSite},
		{"IsVisible", func() error { _, err := plan.IsVisible(vega, start, nil, angle.Deg(10)); return err }, plan.ErrNilSite},
		{"VisibleIntervals", func() error {
			_, err := plan.VisibleIntervals(vega, nil, start, end, 5*time.Minute, angle.Deg(10))
			return err
		}, plan.ErrNilSite},
		{"TransitEstimate", func() error { _, _, err := plan.TransitEstimate(vega, nil, start, end); return err }, plan.ErrNilSite},
		{"Find", func() error { _, err := plan.Find(vega, nil, nil, start, end, 5*time.Minute); return err }, plan.ErrNilSite},
		{"ObservableWindows", func() error {
			_, err := plan.ObservableWindows(star, start, end, unit.Minutes(5), nil)
			return err
		}, plan.ErrNilSite},
		{"SatellitePasses", func() error {
			_, err := plan.SatellitePasses(eph.Default(), eph.Moon, "Moon", start, end, nil, angle.Deg(10))
			return err
		}, coord.ErrNilGeodetic},
	} {
		if err := noPanic(c.call); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
}
