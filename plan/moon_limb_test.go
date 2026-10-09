package plan_test

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestMoonEventsPutTheUpperLimbOnTheHorizon is #693, tested on the property
// itself rather than against an almanac that prints whole minutes: at every
// moonrise and moonset, the Moon's upper limb, its center's geometric
// altitude raised by its semi-diameter at that instant, crosses the
// almanac horizon, RiseSetThreshold, within two seconds of the reported time.
//
// MoonEvents used to hold the Moon to a fixed mean semi-diameter of 15.5′
// while its own runs from 14.7′ to 16.8′, so the limb crossed the horizon up
// to 12 s from the reported instant at London and 28 s at 60°N, inside
// USNO's rounding and so invisible to the USNO tests. The year is checked to
// span most of that range, so the test cannot pass on a month of Moons near
// the mean.
func TestMoonEventsPutTheUpperLimbOnTheHorizon(t *testing.T) {
	t.Parallel()

	prov := eph.Default()
	moon := plan.NewMoon(prov)

	for _, s := range []struct {
		name     string
		lat, lon float64
	}{{"London", 51.5074, -0.1278}, {"60N", 60, 10}} {
		loc, err := coord.NewGeodetic(angle.Deg(s.lon), angle.Deg(s.lat), 0)
		if err != nil {
			t.Fatalf("NewGeodetic: %v", err)
		}

		site, err := plan.NewSite(s.name, loc)
		if err != nil {
			t.Fatalf("NewSite: %v", err)
		}

		start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

		events, err := plan.MoonEvents(start, start.Add(unit.Days(365)), site, prov)
		if err != nil {
			t.Fatalf("%s: MoonEvents: %v", s.name, err)
		}

		horizon := site.RiseSetThreshold().Degrees()

		// limbAbove is the upper limb's height above the horizon at t, in
		// degrees, and the semi-diameter, in arcminutes, from a fresh
		// geometric Context.
		limbAbove := func(at time.Time) (float64, float64) {
			ctx := coord.NewContext(at, loc, atmosphere.Refraction{})

			vec, err := moon.GeocentricVec(at)
			if err != nil {
				t.Fatalf("%s: GeocentricVec: %v", s.name, err)
			}

			d, err := plan.AngularDiameter(moon, at, ctx)
			if err != nil {
				t.Fatalf("%s: AngularDiameter: %v", s.name, err)
			}

			sd := d.Degrees() / 2

			return ctx.GeocentricToObserved(vec).Alt().Degrees() + sd - horizon, sd * 60
		}

		minSD, maxSD, n := 99.0, 0.0, 0

		for _, ev := range events {
			if ev.Kind != plan.EventRise && ev.Kind != plan.EventSet {
				continue
			}

			before, sd := limbAbove(ev.Time.Add(unit.Seconds(-2)))
			after, _ := limbAbove(ev.Time.Add(unit.Seconds(2)))

			if (before > 0) == (after > 0) {
				t.Errorf("%s: %v reported at %v, but the upper limb does not cross the horizon within 2 s of it "+
					"(%.5f° then %.5f°, semi-diameter %.2f′)", s.name, ev.Kind, ev.Time, before, after, sd)
			}

			minSD, maxSD, n = min(minSD, sd), max(maxSD, sd), n+1
		}

		if n < 600 {
			t.Fatalf("%s: %d rises and sets over the year; the fixture no longer covers it", s.name, n)
		}

		if maxSD-minSD < 1.5 {
			t.Errorf("%s: semi-diameters %.2f′–%.2f′ over the year; the test needs the Moon near perigee "+
				"and apogee to tell a fixed semi-diameter from the real one", s.name, minSD, maxSD)
		}
	}
}

// TestUpperLimbNeedsABody: a target with no distance has no semi-diameter to
// raise its altitude by, and the spec says so before any search.
func TestUpperLimbNeedsABody(t *testing.T) {
	t.Parallel()

	loc, err := coord.NewGeodetic(angle.Deg(0), angle.Deg(51.5), 0)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	site, err := plan.NewSite("s", loc)
	if err != nil {
		t.Fatalf("NewSite: %v", err)
	}

	spec := plan.EventSpec{
		Family:    plan.EventFamilyVisibility,
		Kind:      plan.EventAnyVisibility,
		Target:    plan.NewStar("Sirius", angle.Deg(101.29), angle.Deg(-16.72)),
		Observer:  site,
		Threshold: site.RiseSetThreshold(),
		UpperLimb: true,
	}

	if err := spec.Validate(); !errors.Is(err, plan.ErrUpperLimbNeedsBody) {
		t.Errorf("Validate with UpperLimb on a star: %v, want ErrUpperLimbNeedsBody", err)
	}
}
