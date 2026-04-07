package plan

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/testutil"

	"github.com/TuSKan/astrogo/time"
)

// ── Generic Event Finder Tests ──────────────────────────────────────────────

func TestEventFinder_Fixed(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := NewSite("Test", loc, angle.Zero(), nil)
	obj := Custom{Coord: coord.NewICRS(angle.Deg(0), angle.Deg(0))}

	start := time.FromJD(2451545.0, time.UTC)
	end := start.Add(24 * time.Hour)

	finder := NewEventFinder(30*time.Minute, 1*time.Second)
	events, err := finder.FindEvents(obj, start, end, site, angle.Deg(20))
	testutil.AssertNoError(t, err)

	if len(events) == 0 {
		t.Error("expected at least one event")
	}

	for i, e := range events {
		t.Log(e.String())
		if i > 0 {
			if e.Time.Before(events[i-1].Time) {
				t.Errorf("events not sorted: %v before %v", e.Time, events[i-1].Time)
			}
		}

		if e.Kind == EventRise || e.Kind == EventSet {
			testutil.AssertNear(t, "altitude", e.GeometricAltitude.Degrees(), 20.0, 0.01)
		}
	}
}

func TestEventFinder_Circumpolar(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := NewSite("Test", loc, angle.Zero(), nil)
	obj := Custom{Coord: coord.NewICRS(angle.Deg(0), angle.Deg(80))}

	start := time.FromJD(2451545.0, time.UTC)
	end := start.Add(24 * time.Hour)

	finder := NewEventFinder(30*time.Minute, 10*time.Second)
	events, err := finder.FindEvents(obj, start, end, site, angle.Deg(10))
	testutil.AssertNoError(t, err)

	for _, e := range events {
		if e.Kind == EventRise || e.Kind == EventSet {
			t.Errorf("unexpected rise/set for circumpolar target: %v", e)
		}
	}
}

func TestEventFinder_NeverVisible(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := NewSite("Test", loc, angle.Zero(), nil)
	obj := Custom{Coord: coord.NewICRS(angle.Deg(0), angle.Deg(-80))}

	start := time.FromJD(2451545.0, time.UTC)
	end := start.Add(24 * time.Hour)

	finder := NewEventFinder(30*time.Minute, 10*time.Second)
	events, err := finder.FindEvents(obj, start, end, site, angle.Deg(0))
	testutil.AssertNoError(t, err)

	for _, e := range events {
		if e.Kind == EventRise || e.Kind == EventSet {
			t.Errorf("unexpected rise/set for never-visible target: %v", e)
		}
	}
}

// ── Sun and Moon Helper Tests ──────────────────────────────────────────────

func TestSunEvents(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(40), 0)
	site, _ := NewSite("Test", loc, angle.Zero(), nil)
	eph := ephemeris.Default()

	start := time.FromJD(2451544.5, time.UTC)
	end := start.Add(24 * time.Hour)

	events, err := SunEvents(start, end, site, eph)
	testutil.AssertNoError(t, err)

	hasRise, hasSet, hasTransit := false, false, false
	for _, e := range events {
		switch e.Kind {
		case EventRise:
			hasRise = true
			testutil.AssertNear(t, "sunrise altitude", e.GeometricAltitude.Degrees(), SunHorizonAltitude, 0.01)
		case EventSet:
			hasSet = true
			testutil.AssertNear(t, "sunset altitude", e.GeometricAltitude.Degrees(), SunHorizonAltitude, 0.01)
		case EventTransit:
			hasTransit = true
		}
	}

	if !hasRise || !hasSet || !hasTransit {
		t.Errorf("missing Sun events: rise=%v, set=%v, transit=%v", hasRise, hasSet, hasTransit)
	}
}

func TestSunEvents_Polar(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(80), 0)
	site, _ := NewSite("Arctic", loc, angle.Zero(), nil)
	eph := ephemeris.Default()

	start := time.FromJD(2451727.5, time.UTC)
	end := start.Add(24 * time.Hour)

	events, err := SunEvents(start, end, site, eph)
	testutil.AssertNoError(t, err)

	for _, e := range events {
		if e.Kind == EventRise || e.Kind == EventSet {
			t.Errorf("unexpected rise/set during Midnight Sun: %v", e)
		}
	}
}

func TestMoonEvents(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(40), 0)
	site, _ := NewSite("Test", loc, angle.Zero(), nil)
	eph := ephemeris.Default()

	start := time.FromJD(2451545.0, time.UTC)
	end := start.Add(24 * time.Hour)

	events, err := MoonEvents(start, end, site, eph)
	testutil.AssertNoError(t, err)

	for _, e := range events {
		if e.Kind == EventRise || e.Kind == EventSet {
			testutil.AssertNear(t, "moonrise/set altitude", e.GeometricAltitude.Degrees(), MoonHorizonAltitude, 0.01)
		}
	}
}

func TestSunriseSunset(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(40), 0)
	site, _ := NewSite("Test", loc, angle.Zero(), nil)
	eph := ephemeris.Default()

	start := time.FromJD(2451544.5, time.UTC)
	end := start.Add(24 * time.Hour)

	rise, set, err := SunriseSunset(start, end, site, eph)
	testutil.AssertNoError(t, err)

	if rise == nil || set == nil {
		t.Errorf("expected sunrise and sunset, got rise=%v, set=%v", rise, set)
	}
}

// ── Twilight Tests ──────────────────────────────────────────────────────────

func TestTwilightEvents(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(40), 0)
	site, _ := NewSite("Test", loc, angle.Zero(), nil)
	eph := ephemeris.Default()

	start := time.FromJD(2451544.5, time.UTC)
	end := start.Add(24 * time.Hour)

	kinds := []TwilightKind{
		CivilTwilight,
		NauticalTwilight,
		AstronomicalTwilight,
	}

	for _, kind := range kinds {
		t.Run(kind.String(), func(t *testing.T) {
			events, err := TwilightEvents(start, end, site, eph, kind)
			testutil.AssertNoError(t, err)

			if len(events) == 0 {
				t.Fatalf("no twilight events found for %s", kind)
			}

			for _, e := range events {
				if e.Dawn != nil {
					testutil.AssertNear(t, "dawn altitude", e.Dawn.GeometricAltitude.Degrees(), TwilightThresholds[kind], 0.01)
				}
				if e.Dusk != nil {
					testutil.AssertNear(t, "dusk altitude", e.Dusk.GeometricAltitude.Degrees(), TwilightThresholds[kind], 0.01)
				}
			}
		})
	}
}

func TestTwilight_Sequence(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(40), 0)
	site, _ := NewSite("Test", loc, angle.Zero(), nil)
	eph := ephemeris.Default()

	start := time.FromJD(2451544.5, time.UTC)
	end := start.Add(24 * time.Hour)

	aDawn, aDusk, _ := AstronomicalDawnDusk(start, end, site, eph)
	nDawn, nDusk, _ := NauticalDawnDusk(start, end, site, eph)
	cDawn, cDusk, _ := CivilDawnDusk(start, end, site, eph)
	rise, set, _ := SunriseSunset(start, end, site, eph)

	if !aDawn.Time.Before(nDawn.Time) {
		t.Errorf("Astro dawn should be before Nautical dawn: %v vs %v", aDawn.Time, nDawn.Time)
	}
	if !nDawn.Time.Before(cDawn.Time) {
		t.Errorf("Nautical dawn should be before Civil dawn: %v vs %v", nDawn.Time, cDawn.Time)
	}
	if !cDawn.Time.Before(rise.Time) {
		t.Errorf("Civil dawn should be before Sunrise: %v vs %v", cDawn.Time, rise.Time)
	}

	if !set.Time.Before(cDusk.Time) {
		t.Errorf("Sunset should be before Civil dusk: %v vs %v", set.Time, cDusk.Time)
	}
	if !cDusk.Time.Before(nDusk.Time) {
		t.Errorf("Civil dusk should be before Nautical dusk: %v vs %v", cDusk.Time, nDusk.Time)
	}
	if !nDusk.Time.Before(aDusk.Time) {
		t.Errorf("Nautical dusk should be before Astro dusk: %v vs %v", nDusk.Time, aDusk.Time)
	}
}

func TestTwilight_HighLat(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(51.5), 0)
	site, _ := NewSite("London", loc, angle.Zero(), nil)
	eph := ephemeris.Default()

	start := time.FromJD(2451727.5, time.UTC)
	end := start.Add(24 * time.Hour)

	aDawn, aDusk, err := AstronomicalDawnDusk(start, end, site, eph)
	testutil.AssertNoError(t, err)

	if aDawn != nil || aDusk != nil {
		t.Errorf("expected no astronomical twilight in London summer, got dawn=%v dusk=%v", aDawn, aDusk)
	}
}

// ── Benchmarks ─────────────────────────────────────────────────────────────

func BenchmarkEventFinder(b *testing.B) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := NewSite("Test", loc, angle.Zero(), nil)
	obj := Custom{Coord: coord.NewICRS(angle.Deg(0), angle.Deg(0))}
	start := time.FromJD(2451545.0, time.UTC)
	end := start.Add(24 * time.Hour)
	finder := NewEventFinder(30*time.Minute, 1*time.Second)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = finder.FindEvents(obj, start, end, site, angle.Deg(20))
	}
}
