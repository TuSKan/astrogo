package plan

import (
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

var errFailingWhen = errors.New("failingWhenProvider: this lookup fails")

// failingWhenProvider fails the lookups fails picks and answers the rest.
type failingWhenProvider struct {
	eph.Provider

	fails func(id eph.ID, t time.Time) bool
}

func (p failingWhenProvider) State(id eph.ID, t time.Time) (eph.State, error) {
	if p.fails(id, t) {
		return eph.State{}, errFailingWhen
	}

	st, err := p.Provider.State(id, t)
	if err != nil {
		return eph.State{}, fmt.Errorf("failingWhenProvider: %w", err)
	}

	return st, nil
}

// countingProvider counts the lookups it passes through.
type countingProvider struct {
	eph.Provider

	calls int
}

func (p *countingProvider) State(id eph.ID, t time.Time) (eph.State, error) {
	p.calls++

	st, err := p.Provider.State(id, t)
	if err != nil {
		return eph.State{}, fmt.Errorf("countingProvider: %w", err)
	}

	return st, nil
}

// A lookup that fails must come back as an error, never as a verdict read
// from a zero position: the Sun's, which every search starts from; the
// Moon's; and each of the last lookups, which the evening's geometry is read
// from after every search has succeeded.
func TestCrescentVisibilityReportsAFailedLookup(t *testing.T) {
	t.Parallel()

	site, evening := portOfSpainEvening(t)

	for _, body := range []eph.ID{eph.Sun, eph.Moon} {
		prov := failingWhenProvider{Provider: eph.Default(), fails: func(id eph.ID, _ time.Time) bool { return id == body }}

		if _, err := CrescentVisibility(evening, site, prov); !errors.Is(err, errFailingWhen) {
			t.Errorf("body %v failing: err = %v, want the lookup's error", body, err)
		}
	}

	counting := &countingProvider{Provider: eph.Default()}

	r, err := CrescentVisibility(evening, site, counting)
	if err != nil {
		t.Fatalf("CrescentVisibility: %v", err)
	}

	// The new-moon search looks only hours back; the sunset and moonset
	// searches only forward.
	phases := failingWhenProvider{Provider: eph.Default(), fails: func(id eph.ID, at time.Time) bool {
		return id == eph.Moon && at.Before(r.Sunset.Add(unit.Hours(-1)))
	}}

	if _, err := CrescentVisibility(evening, site, phases); !errors.Is(err, errFailingWhen) {
		t.Errorf("the new-moon search failing: err = %v, want the lookup's error", err)
	}

	for back := 1; back <= 4; back++ {
		prov := &failingOnceProvider{Provider: eph.Default(), on: counting.calls - back}

		if _, err := CrescentVisibility(evening, site, prov); !errors.Is(err, errFailingOnce) {
			t.Errorf("lookup %d from the end failing: err = %v, want the lookup's error", back, err)
		}
	}
}

// On an evening when the Moon set first, the moonset is found by searching
// back from sunset, and a failure there is reported too.
func TestMoonsetSearchBackReportsAFailedLookup(t *testing.T) {
	t.Parallel()

	site := crescentSite(t, 10.65, -61.52)
	evening := time.Date(2025, 3, 28, 16, 0, 0, 0, time.LocationUTC)

	r, err := CrescentVisibility(evening, site, eph.Default())
	if err != nil {
		t.Fatalf("CrescentVisibility: %v", err)
	}

	if !r.Moonset.Before(r.Sunset) {
		t.Fatalf("fixture moved: moonset %v is not before sunset %v", r.Moonset, r.Sunset)
	}

	// A minute back: the Moon's apparent place at sunset is looked up a
	// light-time, 1.3 s, earlier, and that lookup belongs to the search
	// forward.
	before := failingWhenProvider{Provider: eph.Default(), fails: func(id eph.ID, at time.Time) bool {
		return id == eph.Moon && at.Before(r.Sunset.Add(unit.Minutes(-1)))
	}}

	if _, err := moonsetFor(r.Sunset, site, before); !errors.Is(err, errFailingWhen) {
		t.Errorf("err = %v, want the failed lookup before sunset", err)
	}
}

func TestMoonAgeHoursReportsFailures(t *testing.T) {
	t.Parallel()

	at := time.Date(2025, 3, 29, 22, 0, 0, 0, time.LocationUTC)

	// The elongation at the instant itself, and the phase search behind it.
	for _, c := range []struct {
		name  string
		fails func(eph.ID, time.Time) bool
	}{
		{"elongation", func(id eph.ID, _ time.Time) bool { return id == eph.Moon }},
		{"phase search", func(id eph.ID, t time.Time) bool { return id == eph.Moon && t.Before(at.Add(unit.Hours(-1))) }},
	} {
		prov := failingWhenProvider{Provider: eph.Default(), fails: c.fails}

		if _, err := moonAgeHours(at, prov); !errors.Is(err, errFailingWhen) {
			t.Errorf("%s failing: err = %v, want the lookup's error", c.name, err)
		}
	}

	// A Moon held 100° east of the Sun on the ecliptic never laps it, so the
	// bracket the elongation draws holds no new moon.
	const obliquity = 23.44 * math.Pi / 180

	lon := 100 * math.Pi / 180
	stuck := twoBodyProvider{
		sun:  vector.Vec3{X: 1},
		moon: vector.Vec3{X: math.Cos(lon), Y: math.Sin(lon) * math.Cos(obliquity), Z: math.Sin(lon) * math.Sin(obliquity)}.MulScalar(0.00257),
	}

	if _, err := moonAgeHours(at, stuck); !errors.Is(err, errNoNewMoon) {
		t.Errorf("a Moon that never laps the Sun: err = %v, want errNoNewMoon", err)
	}
}

// Three hours after a new moon the elongation is under 4°, and the bracket's
// late end, E/15 days back less the margin, falls after the instant itself;
// it is held at the instant.
func TestMoonAgeHoursJustAfterANewMoon(t *testing.T) {
	t.Parallel()

	at := time.Date(2025, 3, 29, 13, 58, 0, 0, time.LocationUTC)

	age, err := moonAgeHours(at, eph.Default())
	if err != nil {
		t.Fatalf("moonAgeHours: %v", err)
	}

	if want := ageFromALunation(t, at, eph.Default()); math.Abs(age-want) > 1.0/3600 {
		t.Errorf("age %.6f h, a lunation's search %.6f h", age, want)
	}

	if age < 2.5 || age > 3.5 {
		t.Errorf("age %.3f h three hours after the new moon", age)
	}
}

// twoBodyProvider puts the Sun and the Moon at fixed geocentric positions.
type twoBodyProvider struct {
	sun, moon vector.Vec3
}

func (p twoBodyProvider) State(id eph.ID, _ time.Time) (eph.State, error) {
	if id == eph.Moon {
		return eph.State{Pos: p.moon}, nil
	}

	return eph.State{Pos: p.sun}, nil
}

func (twoBodyProvider) Close() error { return nil }

// A span that is not a whole number of chunks ends in a short one, and an
// event there is found; a search that fails is reported.
func TestEventSearchesEndInAShortChunk(t *testing.T) {
	t.Parallel()

	start := time.Date(2025, 3, 29, 0, 0, 0, 0, time.LocationUTC)
	end := start.Add(unit.Days(0.3)) // 7.2 h: one whole chunk and a short one

	var windows [][2]time.Time

	// The event sits in the short chunk: 6.6 h after start, 0.6 h after end − 7.2 h.
	searchAt := func(event time.Time) eventsFunc {
		return func(from, to time.Time, _ *Site, _ eph.Provider) ([]Event, error) {
			windows = append(windows, [2]time.Time{from, to})
			if !event.Before(from) && !event.After(to) {
				return []Event{{Kind: EventSet, Time: event}}, nil
			}

			return nil, nil
		}
	}

	windows = nil

	e, found, err := firstEvent(searchAt(start.Add(unit.Hours(6.6))), start, 0.3, nil, nil, isSet)
	if err != nil || !found || len(windows) != 2 || !windows[1][1].Equal(end) {
		t.Errorf("firstEvent: found %v at %v, err %v, windows %v; want the event in a last window ending at %v", found, e.Time, err, windows, end)
	}

	windows = nil

	e, found, err = lastEvent(searchAt(start.Add(unit.Hours(0.6))), end, 0.3, nil, nil, isSet)
	if err != nil || !found || len(windows) != 2 || !windows[1][0].Equal(start) {
		t.Errorf("lastEvent: found %v at %v, err %v, windows %v; want the event in a last window starting at %v", found, e.Time, err, windows, start)
	}

	broken := func(time.Time, time.Time, *Site, eph.Provider) ([]Event, error) { return nil, errFailingWhen }

	if _, _, err := firstEvent(broken, start, 0.3, nil, nil, isSet); !errors.Is(err, errFailingWhen) {
		t.Errorf("firstEvent over a failing search: err = %v", err)
	}

	if _, _, err := lastEvent(broken, end, 0.3, nil, nil, isSet); !errors.Is(err, errFailingWhen) {
		t.Errorf("lastEvent over a failing search: err = %v", err)
	}
}

// The difference in azimuth across north is the short way round.
func TestAzimuthGapAcrossNorth(t *testing.T) {
	t.Parallel()

	for _, c := range []struct{ a, b, want float64 }{{350, 10, 20}, {10, 350, 20}, {90, 270, 180}, {100, 120, 20}} {
		got := azimuthGapDeg(coord.NewAltAz(0, angle.Deg(c.a)), coord.NewAltAz(0, angle.Deg(c.b)))
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("gap between %g° and %g° = %g°, want %g°", c.a, c.b, got, c.want)
		}
	}
}

// crescentGeometryAt reports a failed lookup of either body.
func TestCrescentGeometryReportsAFailedLookup(t *testing.T) {
	t.Parallel()

	site, evening := portOfSpainEvening(t)
	ctx := coord.NewContext(evening, site.Location(), atmosphere.Refraction{})

	for _, body := range []eph.ID{eph.Sun, eph.Moon} {
		prov := failingWhenProvider{Provider: eph.Default(), fails: func(id eph.ID, _ time.Time) bool { return id == body }}

		if _, err := crescentGeometryAt(ctx, prov); !errors.Is(err, errFailingWhen) {
			t.Errorf("body %v failing: err = %v", body, err)
		}
	}
}
