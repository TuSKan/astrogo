package plan

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/testutil"

	"github.com/TuSKan/astrogo/time"
)

// ── Generic Event Finder Tests ──────────────────────────────────────────────

func TestEventSolver_Visibility_Fixed(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := NewSite("Test", loc, angle.Zero(), nil)
	obj := Custom{Coord: coord.NewICRS(angle.Deg(0), angle.Deg(0))}

	start := time.FromJD(2451545.0, time.UTC)
	end := start.Add(24 * time.Hour)

	solver := NewEventSolver(30*time.Minute, 1*time.Second)
	events, err := solver.Find(EventSpec{
		Family:    EventFamilyVisibility,
		Kind:      EventAnyVisibility,
		Target:    obj,
		Observer:  site,
		Threshold: angle.Deg(20),
	}, start, end)
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

func TestEventSolver_Visibility_Circumpolar(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := NewSite("Test", loc, angle.Zero(), nil)
	obj := Custom{Coord: coord.NewICRS(angle.Deg(0), angle.Deg(80))}

	start := time.FromJD(2451545.0, time.UTC)
	end := start.Add(24 * time.Hour)

	solver := NewEventSolver(30*time.Minute, 10*time.Second)
	events, err := solver.Find(EventSpec{
		Family:    EventFamilyVisibility,
		Kind:      EventAnyVisibility,
		Target:    obj,
		Observer:  site,
		Threshold: angle.Deg(10),
	}, start, end)
	testutil.AssertNoError(t, err)

	for _, e := range events {
		if e.Kind == EventRise || e.Kind == EventSet {
			t.Errorf("unexpected rise/set for circumpolar target: %v", e)
		}
	}
}

func TestEventSolver_Visibility_NeverVisible(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := NewSite("Test", loc, angle.Zero(), nil)
	obj := Custom{Coord: coord.NewICRS(angle.Deg(0), angle.Deg(-80))}

	start := time.FromJD(2451545.0, time.UTC)
	end := start.Add(24 * time.Hour)

	solver := NewEventSolver(30*time.Minute, 10*time.Second)
	events, err := solver.Find(EventSpec{
		Family:    EventFamilyVisibility,
		Kind:      EventAnyVisibility,
		Target:    obj,
		Observer:  site,
		Threshold: angle.Deg(0),
	}, start, end)
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

func BenchmarkEventSolver(b *testing.B) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := NewSite("Test", loc, angle.Zero(), nil)
	obj := Custom{Coord: coord.NewICRS(angle.Deg(0), angle.Deg(0))}
	start := time.FromJD(2451545.0, time.UTC)
	end := start.Add(24 * time.Hour)
	solver := NewEventSolver(30*time.Minute, 1*time.Second)
	spec := EventSpec{
		Family:    EventFamilyVisibility,
		Kind:      EventAnyVisibility,
		Target:    obj,
		Observer:  site,
		Threshold: angle.Deg(20),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = solver.Find(spec, start, end)
	}
}

// ── Geometry Tests ─────────────────────────────────────────────────────────

// mockLinearTarget sweeps across Right Ascension linearly.
type mockLinearTarget struct {
	raRate  float64 // deg per hour
	startRA float64
	dec     float64
}

func (m *mockLinearTarget) Position(t time.Time) (*coord.ICRS, error) {
	hours := float64(t.Sub(time.FromJD(2451545.0, time.UTC)).Hours())
	ra := m.startRA + m.raRate*hours
	// Normalize RA
	for ra >= 360 {
		ra -= 360
	}
	for ra < 0 {
		ra += 360
	}
	return coord.NewICRS(angle.Deg(ra), angle.Deg(m.dec)), nil
}

func (m *mockLinearTarget) Constraints() []Constraint { return nil }
func (m *mockLinearTarget) Catalog() string           { return "MOCK" }
func (m *mockLinearTarget) ID() string                { return "Linear" }
func (m *mockLinearTarget) Name() string              { return "LinearName" }

func TestSolveGeometry_Conjunction(t *testing.T) {
	t1 := &mockLinearTarget{raRate: 1.0, startRA: 10, dec: 0.0}
	t2 := &mockLinearTarget{raRate: 0.5, startRA: 15, dec: 0.0}

	start := time.FromJD(2451545.0, time.UTC)
	end := start.Add(24 * time.Hour)

	solver := NewEventSolver(1*time.Hour, 1*time.Second)

	spec := EventSpec{
		Family: EventFamilyRelativeGeometry,
		Kind:   EventConjunction,
		Target: t1,
		Other:  t2,
	}

	events, err := solver.Find(spec, start, end)
	testutil.AssertNoError(t, err)

	if len(events) != 1 {
		t.Fatalf("expected 1 conjunction event, got %d", len(events))
	}

	event := events[0]
	if event.Kind != EventConjunction {
		t.Errorf("expected EventConjunction, got %v", event.Kind)
	}

	gotHours := float64(event.Time.Sub(start).Hours())
	testutil.AssertNear(t, "conjunction time", gotHours, 10.0, 0.01)
}

func TestSolveGeometry_Opposition(t *testing.T) {
	t1 := &mockLinearTarget{raRate: 1.0, startRA: 175, dec: 0.0}
	t2 := &mockLinearTarget{raRate: 0.0, startRA: 0, dec: 0.0}

	start := time.FromJD(2451545.0, time.UTC)
	end := start.Add(10 * time.Hour)

	solver := NewEventSolver(1*time.Hour, 1*time.Second)

	spec := EventSpec{
		Family: EventFamilyRelativeGeometry,
		Kind:   EventOpposition,
		Target: t1,
		Other:  t2,
	}

	events, err := solver.Find(spec, start, end)
	testutil.AssertNoError(t, err)

	if len(events) != 1 {
		t.Fatalf("expected 1 opposition event, got %d", len(events))
	}

	gotHours := float64(events[0].Time.Sub(start).Hours())
	testutil.AssertNear(t, "opposition time", gotHours, 5.0, 0.01)
}

// target with parabolic separation distance to test Greatest Elongation.
type mockParabolicTarget struct {
	a float64
	h float64
	k float64
}

func (m *mockParabolicTarget) Position(t time.Time) (*coord.ICRS, error) {
	hours := float64(t.Sub(time.FromJD(2451545.0, time.UTC)).Hours())
	dec := m.a*math.Pow(hours-m.h, 2) + m.k
	return coord.NewICRS(angle.Deg(0), angle.Deg(dec)), nil
}

func (m *mockParabolicTarget) Constraints() []Constraint { return nil }
func (m *mockParabolicTarget) Catalog() string           { return "MOCK" }
func (m *mockParabolicTarget) ID() string                { return "Para" }
func (m *mockParabolicTarget) Name() string              { return "ParaName" }

func TestSolveGeometry_GreatestElongation(t *testing.T) {
	t2 := &mockLinearTarget{raRate: 0, startRA: 10, dec: 0}

	t1 := &mockParabolicTarget{
		a: -1.0,
		h: 6.0,
		k: 15.0,
	}

	t1_pos := func(t time.Time) (*coord.ICRS, error) {
		hours := float64(t.Sub(time.FromJD(2451545.0, time.UTC)).Hours())
		dec := t1.a*math.Pow(hours-t1.h, 2) + t1.k
		return coord.NewICRS(angle.Deg(20), angle.Deg(dec)), nil
	}

	wrapper := &mockDynamicTarget{f: t1_pos}

	start := time.FromJD(2451545.0, time.UTC)
	end := start.Add(12 * time.Hour)

	solver := NewEventSolver(1*time.Hour, 1*time.Second)

	spec := EventSpec{
		Family: EventFamilyRelativeGeometry,
		Kind:   EventGreatestElongationEast,
		Target: wrapper,
		Other:  t2,
	}

	events, err := solver.Find(spec, start, end)
	testutil.AssertNoError(t, err)

	if len(events) != 1 {
		t.Fatalf("expected 1 greatest elongation event, got %d", len(events))
	}

	gotHours := float64(events[0].Time.Sub(start).Hours())
	testutil.AssertNear(t, "elongation time", gotHours, 6.0, 0.1)
}

type mockDynamicTarget struct {
	f func(t time.Time) (*coord.ICRS, error)
}

func (m *mockDynamicTarget) Position(t time.Time) (*coord.ICRS, error) { return m.f(t) }
func (m *mockDynamicTarget) Constraints() []Constraint                 { return nil }
func (m *mockDynamicTarget) Catalog() string                           { return "DYN" }
func (m *mockDynamicTarget) ID() string                                { return "Dyn" }
func (m *mockDynamicTarget) Name() string                              { return "DynName" }
