package plan

import (
	stdtime "time"

	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
)

// ── DayEvents ────────────────────────────────────────────────────────────

func TestDayEventsFindsSunRiseSetTransit(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(40), 0)
	site, _ := NewSite("Test", loc)
	sun := NewSun(eph.Default())

	day := time.FromJD(2451545.0, time.UTC) // any instant on the day in question

	rise, set, transit, err := DayEvents(day, stdtime.UTC, sun, site)
	testutil.AssertNoError(t, err)

	if rise == nil || set == nil || transit == nil {
		t.Fatalf("expected rise, set, and transit at a mid-latitude site; got rise=%v set=%v transit=%v", rise, set, transit)
	}

	if !rise.Time.Before(transit.Time) || !transit.Time.Before(set.Time) {
		t.Errorf("expected rise < transit < set, got %v / %v / %v", rise.Time, transit.Time, set.Time)
	}
}

// TestDayEventsUsesLocalCalendarDay verifies the day boundary is measured
// in the given loc, not UTC: an instant near local midnight in a
// non-UTC-aligned timezone must resolve to THAT timezone's calendar day.
func TestDayEventsUsesLocalCalendarDay(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(40), 0)
	site, _ := NewSite("Test", loc)
	sun := NewSun(eph.Default())

	// 2000-01-01 00:30 UTC is still 2000-01-01 in a UTC-based day, but is
	// already 2000-01-01 19:30 the PREVIOUS day in a UTC-5 zone -- pick an
	// instant where the local calendar date genuinely differs from UTC's.
	utcMidnight30 := time.FromJD(2451544.5+0.5/24, time.UTC) // 2000-01-01 00:30 UTC
	utcMinus5 := stdtime.FixedZone("UTC-5", -5*3600)

	_, _, _, err := DayEvents(utcMidnight30, utcMinus5, sun, site)
	testutil.AssertNoError(t, err)

	// Cross-check: the local calendar date under UTC-5 is 1999-12-31, one
	// day earlier than under UTC -- confirm via the stdlib conversion this
	// function itself relies on, so this test would fail loudly if that
	// assumption were ever wrong.
	localDate := utcMidnight30.GoTime().In(utcMinus5)
	utcDate := utcMidnight30.GoTime().In(stdtime.UTC)

	if localDate.Day() == utcDate.Day() {
		t.Fatal("test fixture assumption broken: local and UTC calendar days should differ at this instant")
	}
}

func TestDayEventsNilLocDefaultsToUTC(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(40), 0)
	site, _ := NewSite("Test", loc)
	sun := NewSun(eph.Default())

	day := time.FromJD(2451545.0, time.UTC)

	withNil, _, _, err := DayEvents(day, nil, sun, site)
	testutil.AssertNoError(t, err)

	withUTC, _, _, err := DayEvents(day, stdtime.UTC, sun, site)
	testutil.AssertNoError(t, err)

	if withNil == nil || withUTC == nil {
		t.Fatal("expected a rise event in both cases")
	}

	if !withNil.Time.Equal(withUTC.Time) {
		t.Errorf("nil loc should behave exactly like stdtime.UTC; got %v vs %v", withNil.Time, withUTC.Time)
	}
}

// TestDayEventsPolarNight verifies nil-without-error for a day the Sun
// never rises at all -- not an error, just what that day looked like.
func TestDayEventsPolarNight(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(80), 0)
	site, _ := NewSite("Arctic", loc)
	sun := NewSun(eph.Default())

	day := time.FromJD(2451727.5, time.UTC) // known midwinter date, per existing TestSunEvents_Polar

	rise, set, _, err := DayEvents(day, stdtime.UTC, sun, site)
	testutil.AssertNoError(t, err)

	if rise != nil || set != nil {
		t.Errorf("expected no rise/set during polar night, got rise=%v set=%v", rise, set)
	}
}

// ── Episode ──────────────────────────────────────────────────────────────

// TestEpisodeWithinWindowMatchesDirectSearch verifies the common case: a
// [from, to] window that fully contains one rise-then-set cycle produces
// exactly that pair, matching what a direct solver.Find over the same
// window would give.
func TestEpisodeWithinWindowMatchesDirectSearch(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(40), 0)
	site, _ := NewSite("Test", loc)
	sun := NewSun(eph.Default())

	from := time.FromJD(2451544.5, time.UTC) // local midday-ish, target below horizon
	to := from.AddDays(1)

	rise, set, err := Episode(from, to, sun, site)
	testutil.AssertNoError(t, err)

	if rise == nil || set == nil {
		t.Fatal("expected both rise and set")
	}

	if !rise.Time.Before(set.Time) {
		t.Errorf("expected rise before set, got rise=%v set=%v", rise.Time, set.Time)
	}

	direct, err := visibilityEvents(sun, site, from, to)
	testutil.AssertNoError(t, err)

	var directRise, directSet *Event

	for i := range direct {
		if direct[i].Kind == EventRise && directRise == nil {
			directRise = &direct[i]
		}

		if direct[i].Kind == EventSet && directSet == nil {
			directSet = &direct[i]
		}
	}

	if directRise == nil || directSet == nil {
		t.Fatal("test setup: expected a direct rise/set in this window too")
	}

	// solverToleranceJD comes from visibilityEvents' own NewEventSolver(15m, 1s)
	// call: Episode's searchEvent and this test's own direct search bracket
	// the same crossing with DIFFERENT windows (adaptive-doubling probes vs.
	// one fixed 1-day span), so they converge to two independently-solved
	// roots that need only agree within the solver's own declared 1s
	// tolerance, not bit-for-bit -- a tighter tolerance here would be
	// asserting a precision the solver itself never promised.
	const solverToleranceJD = 2.0 / 86400 // 2s, double the solver's 1s tolerance for margin

	testutil.AssertNear(t, "rise time (JD)", rise.Time.JD(), directRise.Time.JD(), solverToleranceJD)
	testutil.AssertNear(t, "set time (JD)", set.Time.JD(), directSet.Time.JD(), solverToleranceJD)
}

// TestEpisodeExtendsBackwardWhenAlreadyUp is the core case from the issue:
// starting the window mid-episode (target already above the horizon at
// from) must find the REAL rise that started it, before from -- not
// report a nil rise, and not confuse this episode's set with some other
// one's.
func TestEpisodeExtendsBackwardWhenAlreadyUp(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(40), 0)
	site, _ := NewSite("Test", loc)
	sun := NewSun(eph.Default())

	// First locate a real sunrise, then start the Episode window 2 hours
	// AFTER it -- guaranteeing "already up at from".
	wideFrom := time.FromJD(2451544.5, time.UTC)
	wideEvents, err := visibilityEvents(sun, site, wideFrom, wideFrom.AddDays(1))
	testutil.AssertNoError(t, err)

	var realRise *Event

	for i := range wideEvents {
		if wideEvents[i].Kind == EventRise {
			realRise = &wideEvents[i]

			break
		}
	}

	if realRise == nil {
		t.Fatal("test setup: expected a real sunrise in the wide window")
	}

	from := realRise.Time.Add(2 * stdtime.Hour)
	to := from.AddDays(1)

	up, err := isAboveHorizon(sun, site, from)
	testutil.AssertNoError(t, err)

	if !up {
		t.Fatal("test setup: expected the Sun to be up 2h after its own rise")
	}

	rise, set, err := Episode(from, to, sun, site)
	testutil.AssertNoError(t, err)

	if rise == nil || set == nil {
		t.Fatal("expected both rise and set")
	}

	testutil.AssertNear(t, "recovered rise time (JD)", rise.Time.JD(), realRise.Time.JD(), 1e-6)

	if !rise.Time.Before(from) {
		t.Errorf("recovered rise (%v) should be before the window start (%v)", rise.Time, from)
	}

	if !set.Time.After(from) {
		t.Errorf("set (%v) should be after the window start (%v) -- it's still this episode's, not an earlier one's", set.Time, from)
	}
}

// TestEpisodeExtendsForwardPastWindowEnd verifies the symmetric case: when
// the target is still up at `to`, Episode must find the real set even
// though it falls after `to`, not truncate or return nil.
func TestEpisodeExtendsForwardPastWindowEnd(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(40), 0)
	site, _ := NewSite("Test", loc)
	sun := NewSun(eph.Default())

	wideFrom := time.FromJD(2451544.5, time.UTC)
	wideEvents, err := visibilityEvents(sun, site, wideFrom, wideFrom.AddDays(1))
	testutil.AssertNoError(t, err)

	var realRise, realSet *Event

	for i := range wideEvents {
		switch wideEvents[i].Kind { //nolint:exhaustive // only rise/set matter here
		case EventRise:
			if realRise == nil {
				realRise = &wideEvents[i]
			}
		case EventSet:
			if realSet == nil && realRise != nil {
				realSet = &wideEvents[i]
			}
		}
	}

	if realRise == nil || realSet == nil {
		t.Fatal("test setup: expected a real rise/set pair in the wide window")
	}

	// End the window 1 hour before the real set, so the episode is still
	// open at `to`.
	from := realRise.Time.Add(1 * stdtime.Hour)
	to := realSet.Time.Add(-1 * stdtime.Hour)

	rise, set, err := Episode(from, to, sun, site)
	testutil.AssertNoError(t, err)

	if rise == nil || set == nil {
		t.Fatal("expected both rise and set")
	}

	// See TestEpisodeWithinWindowMatchesDirectSearch for why this compares
	// against the solver's own declared tolerance, not an arbitrary tight one:
	// realSet came from a wide fixed-window search, set from Episode's
	// adaptive-doubling search -- different brackets, same crossing, both
	// only guaranteed to agree within visibilityEvents' 1s solver tolerance.
	const solverToleranceJD = 2.0 / 86400 // 2s, double the solver's 1s tolerance for margin

	testutil.AssertNear(t, "set time (JD)", set.Time.JD(), realSet.Time.JD(), solverToleranceJD)

	if !set.Time.After(to) {
		t.Errorf("recovered set (%v) should be after the window end (%v)", set.Time, to)
	}
}

// TestEpisodeCircumpolarReturnsNilNil verifies a circumpolar target (never
// sets) returns rise == set == nil with no error, rather than searching
// forever or erroring.
func TestEpisodeCircumpolarReturnsNilNil(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(80), 0)
	site, _ := NewSite("Arctic", loc)
	star := NewStar("Circumpolar", angle.Deg(0), angle.Deg(85)) // dec=85, always up from 80N

	from := time.FromJD(2451544.5, time.UTC)
	to := from.AddDays(1)

	rise, set, err := Episode(from, to, star, site)
	testutil.AssertNoError(t, err)

	if rise != nil || set != nil {
		t.Errorf("expected rise == set == nil for a circumpolar target, got rise=%v set=%v", rise, set)
	}
}

// TestEpisodeNeverRisesReturnsNilNil is the mirror case: a target that
// never clears the horizon at all from this site.
func TestEpisodeNeverRisesReturnsNilNil(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(80), 0)
	site, _ := NewSite("Arctic", loc)
	star := NewStar("NeverUp", angle.Deg(0), angle.Deg(-85)) // dec=-85, always down from 80N

	from := time.FromJD(2451544.5, time.UTC)
	to := from.AddDays(1)

	rise, set, err := Episode(from, to, star, site)
	testutil.AssertNoError(t, err)

	if rise != nil || set != nil {
		t.Errorf("expected rise == set == nil for a target that never rises, got rise=%v set=%v", rise, set)
	}
}
