package plan_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// sunriseSunset returns site's first sunrise and sunset in the 24 hours from
// start.
func sunriseSunset(t *testing.T, site *plan.Site, start time.Time) (rise, set time.Time) {
	t.Helper()

	events, err := plan.SunEvents(start, start.Add(unit.Hours(24)), site, ephemeris.Default())
	if err != nil {
		t.Fatalf("SunEvents: %v", err)
	}

	var haveRise, haveSet bool

	for _, ev := range events {
		switch {
		case ev.Kind == plan.EventRise && !haveRise:
			rise, haveRise = ev.Time, true
		case ev.Kind == plan.EventSet && !haveSet:
			set, haveSet = ev.Time, true
		}
	}

	if !haveRise || !haveSet {
		t.Fatalf("%s: no sunrise or sunset in %v", site.Name(), events)
	}

	return rise, set
}

// TestAHeightAloneDoesNotDipTheHorizon (#1185): a site's height above the
// ellipsoid used to dip its rise and set horizon by a sea horizon's 1.76′·√h,
// always, so a city on a plateau rose the Sun minutes before USNO, whose
// rise and set ignore height. Entered at their real heights, Denver and
// Madrid now rise and set the Sun on USNO's printed minute, retrieved from
// the USNO API (rstt/oneday, 2026-11-14, UTC). With WithHorizonHeight, the
// option for a site that does look down on its horizon, the old dip returns:
// minutes earlier at sunrise and later at sunset.
func TestAHeightAloneDoesNotDipTheHorizon(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 11, 14, 0, 0, 0, 0, time.LocationUTC)
	at := func(h, m int) time.Time { return time.Date(2026, 11, 14, h, m, 0, 0, time.LocationUTC) }

	for _, c := range []struct {
		name                     string
		latDeg, lonDeg, heightM  float64
		usnoRise, usnoSet        time.Time
		dipEarlierAtLeastMinutes float64
	}{
		{"Denver", 39.7392, -104.9903, 1609, at(13, 44), at(23, 45), 6},
		{"Madrid", 40.4168, -3.7038, 667, at(7, 0), at(16, 58), 4},
	} {
		loc, err := coord.NewEarthLocation(c.latDeg, c.lonDeg, c.heightM)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}

		site, err := plan.NewSite(c.name, loc)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}

		if dip := site.HorizonDip(); dip.Radians() != 0 {
			t.Errorf("%s at %g m: dip %v with no height above the horizon, want none", c.name, c.heightM, dip)
		}

		rise, set := sunriseSunset(t, site, start)

		// USNO prints the nearest minute, so the time is within 30 s of it;
		// a minute is the README's claim.
		for _, e := range []struct {
			what      string
			got, usno time.Time
		}{{"sunrise", rise, c.usnoRise}, {"sunset", set, c.usnoSet}} {
			if off := math.Abs(e.got.Sub(e.usno).Seconds()); off > 60 {
				t.Errorf("%s at %g m: %s %s, %.0f s from USNO's %s", c.name, c.heightM, e.what,
					e.got.Format("15:04:05"), off, e.usno.Format("15:04"))
			}
		}

		dipped, err := plan.NewSite(c.name, loc, plan.WithHorizonHeight(unit.Meters(c.heightM)))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}

		dRise, dSet := sunriseSunset(t, dipped, start)
		if earlier := rise.Sub(dRise).Minutes(); earlier < c.dipEarlierAtLeastMinutes {
			t.Errorf("%s with WithHorizonHeight(%g m): sunrise %.1f min earlier, want at least %g",
				c.name, c.heightM, earlier, c.dipEarlierAtLeastMinutes)
		}

		if later := dSet.Sub(set).Minutes(); later < c.dipEarlierAtLeastMinutes {
			t.Errorf("%s with WithHorizonHeight(%g m): sunset %.1f min later, want at least %g",
				c.name, c.heightM, later, c.dipEarlierAtLeastMinutes)
		}
	}
}

// TestWithHorizonHeightRefusesWhatIsNotALength: a negative or non-finite
// height above the horizon would dip every rise and set by a negative amount
// or by NaN without an error.
func TestWithHorizonHeightRefusesWhatIsNotALength(t *testing.T) {
	t.Parallel()

	loc, err := coord.NewEarthLocation(19.8263, -155.47441, 4145)
	if err != nil {
		t.Fatal(err)
	}

	for _, h := range []float64{-1, math.NaN(), math.Inf(1)} {
		if _, err := plan.NewSite("x", loc, plan.WithHorizonHeight(unit.Meters(h))); !errors.Is(err, plan.ErrInvalidHorizonHeight) {
			t.Errorf("WithHorizonHeight(%g m): err = %v, want ErrInvalidHorizonHeight", h, err)
		}
	}

	site, err := plan.NewSite("Mauna Kea over the sea", loc, plan.WithHorizonHeight(unit.Meters(4145)))
	if err != nil {
		t.Fatalf("WithHorizonHeight(4145 m): %v", err)
	}

	// 1.76′·√4145 = 113.3′.
	if got := site.HorizonDip().Degrees() * 60; math.Abs(got-1.76*math.Sqrt(4145)) > 1e-9 {
		t.Errorf("dip from 4145 m above the sea = %.4f′, want 1.76′·√4145", got)
	}

	// Two sites that differ only in the height above their horizon rise and
	// set things at different times, so they are not equal.
	level, err := plan.NewSite("Mauna Kea over the sea", loc)
	if err != nil {
		t.Fatal(err)
	}

	if site.Equal(level) {
		t.Error("a site with a height above its horizon equals one without")
	}
}
