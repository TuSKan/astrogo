package plan

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/internal/testutil"

	"github.com/TuSKan/astrogo/time"
)

// ── Generic Event Finder Tests ──────────────────────────────────────────────

func TestEventSolver_Visibility_Fixed(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(45), 0)
	site, _ := NewSite("Test", loc)
	obj := NewStar("T", angle.Deg(0), angle.Deg(0))

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
	site, _ := NewSite("Test", loc)
	obj := NewStar("T", angle.Deg(0), angle.Deg(80))

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
	site, _ := NewSite("Test", loc)
	obj := NewStar("T", angle.Deg(0), angle.Deg(-80))

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
	site, _ := NewSite("Test", loc)
	eph := eph.Default()

	start := time.FromJD(2451544.5, time.UTC)
	end := start.Add(24 * time.Hour)

	events, err := SunEvents(start, end, site, eph)
	testutil.AssertNoError(t, err)

	hasRise, hasSet, hasTransit := false, false, false

	for _, e := range events {
		switch e.Kind { //nolint:exhaustive // only rise/set/transit tested
		case EventRise:
			hasRise = true

			testutil.AssertNear(t, "sunrise altitude", e.GeometricAltitude.Degrees(), site.SunRiseSetThreshold().Degrees(), 0.01)
		case EventSet:
			hasSet = true

			testutil.AssertNear(t, "sunset altitude", e.GeometricAltitude.Degrees(), site.SunRiseSetThreshold().Degrees(), 0.01)
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
	site, _ := NewSite("Arctic", loc)
	eph := eph.Default()

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
	site, _ := NewSite("Test", loc)
	eph := eph.Default()

	start := time.FromJD(2451545.0, time.UTC)
	end := start.Add(24 * time.Hour)

	events, err := MoonEvents(start, end, site, eph)
	testutil.AssertNoError(t, err)

	for _, e := range events {
		if e.Kind == EventRise || e.Kind == EventSet {
			testutil.AssertNear(t, "moonrise/set altitude", e.GeometricAltitude.Degrees(), site.MoonRiseSetThreshold().Degrees(), 0.01)
		}
	}
}

func TestSunriseSunset(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(40), 0)
	site, _ := NewSite("Test", loc)
	eph := eph.Default()

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
	site, _ := NewSite("Test", loc)
	eph := eph.Default()

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
					testutil.AssertNear(t, "dawn altitude", e.Dawn.GeometricAltitude.Degrees(), TwilightThresholds[kind], 0.02)
				}

				if e.Dusk != nil {
					testutil.AssertNear(t, "dusk altitude", e.Dusk.GeometricAltitude.Degrees(), TwilightThresholds[kind], 0.02)
				}
			}
		})
	}
}

// TestTwilightEventsGroupsDuskWithFollowingDawn is the regression test for
// the documented-but-unimplemented grouping contract: over a normal
// mid-latitude night, TwilightEvents must return exactly ONE fully-paired
// TwilightEvent (both Dawn and Dusk set), with Dusk before Dawn — not two
// half-populated results, which is what the pre-fix implementation
// returned (one per solver event, never both set on the same element).
func TestTwilightEventsGroupsDuskWithFollowingDawn(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(40), 0)
	site, _ := NewSite("Test", loc)
	eph := eph.Default()

	// A window starting in daylight and running long enough to see one
	// full dusk-to-dawn astronomical-twilight span, with margin on both
	// ends so neither edge is truncated.
	start := time.FromJD(2451544.5, time.UTC) // local midday-ish
	end := start.Add(36 * time.Hour)

	events, err := TwilightEvents(start, end, site, eph, AstronomicalTwilight)
	testutil.AssertNoError(t, err)

	var fullyPaired int

	for _, e := range events {
		if e.Dawn != nil && e.Dusk != nil {
			fullyPaired++

			if !e.Dusk.Time.Before(e.Dawn.Time) {
				t.Errorf("paired event: Dusk (%v) should be before Dawn (%v)", e.Dusk.Time, e.Dawn.Time)
			}

			testutil.AssertNear(t, "dusk altitude", e.Dusk.GeometricAltitude.Degrees(), TwilightThresholds[AstronomicalTwilight], 0.02)
			testutil.AssertNear(t, "dawn altitude", e.Dawn.GeometricAltitude.Degrees(), TwilightThresholds[AstronomicalTwilight], 0.02)
		}
	}

	if fullyPaired == 0 {
		t.Fatal("expected at least one fully-paired (Dawn and Dusk both set) TwilightEvent over a 36h mid-latitude window")
	}
}

// TestTwilightEventsEdgeEventsLeftHalfNil verifies the documented edge
// behavior: an event whose partner falls outside [start, end] is still
// returned, with the missing side nil rather than the whole event dropped
// or paired with the wrong neighbor. A window that starts and ends
// mid-twilight (not a clean multiple of a full day) reliably produces
// this at least one edge.
func TestTwilightEventsEdgeEventsLeftHalfNil(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(40), 0)
	site, _ := NewSite("Test", loc)
	eph := eph.Default()

	// First find one real dusk/dawn pair, then shrink the window to start
	// strictly between them -- guaranteeing the leading result in the
	// narrowed window is Dawn-only (its dusk fell before the new start).
	wide := time.FromJD(2451544.5, time.UTC)
	events, err := TwilightEvents(wide, wide.Add(36*time.Hour), site, eph, AstronomicalTwilight)
	testutil.AssertNoError(t, err)

	var pair *TwilightEvent

	for i := range events {
		if events[i].Dawn != nil && events[i].Dusk != nil {
			pair = &events[i]

			break
		}
	}

	if pair == nil {
		t.Fatal("test setup: expected a fully-paired event in the wide window")
	}

	mid := pair.Dusk.Time.Add(pair.Dawn.Time.Sub(pair.Dusk.Time) / 2)

	narrowed, err := TwilightEvents(mid, wide.Add(36*time.Hour), site, eph, AstronomicalTwilight)
	testutil.AssertNoError(t, err)

	if len(narrowed) == 0 {
		t.Fatal("expected at least one event in the narrowed window")
	}

	first := narrowed[0]
	if first.Dusk != nil || first.Dawn == nil {
		t.Errorf("leading event in a window starting mid-twilight should be Dawn-only (Dusk=nil), got Dawn=%v Dusk=%v", first.Dawn, first.Dusk)
	}
}

// TestGroupTwilightEvents exercises groupTwilightEvents directly against
// hand-built event sequences -- including the back-to-back-dusk (and
// back-to-back-dawn) case TwilightEvents' own doc comment describes as
// reachable at high latitude but which real Sun/solver geometry has no
// convenient, reliably-reproducible fixture for.
func TestGroupTwilightEvents(t *testing.T) {
	t0 := time.FromJD(2451544.5, time.UTC)
	dusk := func(h float64) Event {
		return Event{Kind: EventSet, Time: t0.Add(time.Duration(h * float64(time.Hour)))}
	}
	dawn := func(h float64) Event {
		return Event{Kind: EventRise, Time: t0.Add(time.Duration(h * float64(time.Hour)))}
	}

	t.Run("empty input", func(t *testing.T) {
		got := groupTwilightEvents(nil, AstronomicalTwilight)
		if len(got) != 0 {
			t.Errorf("expected no events, got %v", got)
		}
	})

	t.Run("single dusk-dawn pair", func(t *testing.T) {
		got := groupTwilightEvents([]Event{dusk(0), dawn(8)}, AstronomicalTwilight)
		if len(got) != 1 {
			t.Fatalf("expected 1 paired event, got %d: %v", len(got), got)
		}

		if got[0].Dusk == nil || got[0].Dawn == nil {
			t.Errorf("expected both Dusk and Dawn set, got Dusk=%v Dawn=%v", got[0].Dusk, got[0].Dawn)
		}

		if got[0].Kind != AstronomicalTwilight {
			t.Errorf("Kind = %v, want %v", got[0].Kind, AstronomicalTwilight)
		}
	})

	t.Run("leading dawn-only (dusk before start)", func(t *testing.T) {
		got := groupTwilightEvents([]Event{dawn(2)}, AstronomicalTwilight)
		if len(got) != 1 || got[0].Dawn == nil || got[0].Dusk != nil {
			t.Fatalf("expected 1 Dawn-only event, got %v", got)
		}
	})

	t.Run("trailing dusk-only (dawn after end)", func(t *testing.T) {
		got := groupTwilightEvents([]Event{dusk(0)}, AstronomicalTwilight)
		if len(got) != 1 || got[0].Dusk == nil || got[0].Dawn != nil {
			t.Fatalf("expected 1 Dusk-only event, got %v", got)
		}
	})

	// The documented high-latitude edge case: a second dusk arrives with
	// no intervening dawn. The earlier one must be flushed as its own
	// Dusk-only result, not silently overwritten or dropped.
	t.Run("back-to-back dusks flush the earlier one", func(t *testing.T) {
		got := groupTwilightEvents([]Event{dusk(0), dusk(4), dawn(8)}, AstronomicalTwilight)
		if len(got) != 2 {
			t.Fatalf("expected 2 events (flushed dusk-only + paired), got %d: %v", len(got), got)
		}

		if got[0].Dusk == nil || got[0].Dawn != nil {
			t.Errorf("first result should be the flushed, Dusk-only earlier dusk: %v", got[0])
		}

		if !got[0].Dusk.Time.Equal(t0) {
			t.Errorf("flushed dusk should be the FIRST dusk (t=0h), got t=%v", got[0].Dusk.Time)
		}

		if got[1].Dusk == nil || got[1].Dawn == nil {
			t.Errorf("second result should pair the second dusk with the dawn: %v", got[1])
		}
	})

	// Symmetric case: two dawns in a row (no dusk between them) -- neither
	// has a pendingDusk to pair with, so both are independently Dawn-only.
	t.Run("back-to-back dawns are both independently dawn-only", func(t *testing.T) {
		got := groupTwilightEvents([]Event{dawn(0), dawn(4)}, AstronomicalTwilight)
		if len(got) != 2 {
			t.Fatalf("expected 2 independent Dawn-only events, got %d: %v", len(got), got)
		}

		for i, ev := range got {
			if ev.Dawn == nil || ev.Dusk != nil {
				t.Errorf("event %d: expected Dawn-only, got Dawn=%v Dusk=%v", i, ev.Dawn, ev.Dusk)
			}
		}
	})
}

func TestTwilight_Sequence(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Deg(0), angle.Deg(40), 0)
	site, _ := NewSite("Test", loc)
	eph := eph.Default()

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
	site, _ := NewSite("London", loc)
	eph := eph.Default()

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
	site, _ := NewSite("Test", loc)
	obj := NewStar("T", angle.Deg(0), angle.Deg(0))
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

	for b.Loop() {
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

func (m *mockLinearTarget) Position(t time.Time) (coord.ICRS, error) {
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
func (m *mockLinearTarget) GetDetails(_ *coord.Context, _ ...string) (*TargetDetails, error) {
	return &TargetDetails{}, nil
}

// TestEventSolver_Find_UnimplementedFamily is a regression test for R21:
// EventFamilyOverlap passes EventSpec.Validate (it shares the
// RelativeGeometry validation branch) but Find's dispatch switch has no case
// for it — eclipses/occultations are solved via dedicated functions in
// phases.go, not this generic solver. A caller must be able to distinguish
// "not implemented" from other failures via errors.Is against the documented
// public sentinel.
func TestEventSolver_Find_UnimplementedFamily(t *testing.T) {
	t1 := &mockLinearTarget{raRate: 1.0, startRA: 10, dec: 0.0}
	t2 := &mockLinearTarget{raRate: 0.5, startRA: 15, dec: 0.0}

	start := time.FromJD(2451545.0, time.UTC)
	end := start.Add(24 * time.Hour)

	solver := NewEventSolver(1*time.Hour, 1*time.Second)

	_, err := solver.Find(EventSpec{
		Family: EventFamilyOverlap,
		Kind:   EventConjunction,
		Target: t1,
		Other:  t2,
	}, start, end)
	if !errors.Is(err, ErrFamilyNotImpl) {
		t.Errorf("expected ErrFamilyNotImpl for EventFamilyOverlap, got %v", err)
	}
}

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

func (m *mockParabolicTarget) Position(t time.Time) (coord.ICRS, error) {
	hours := float64(t.Sub(time.FromJD(2451545.0, time.UTC)).Hours())
	dec := m.a*(hours-m.h)*(hours-m.h) + m.k

	return coord.NewICRS(angle.Deg(0), angle.Deg(dec)), nil
}

func (m *mockParabolicTarget) Constraints() []Constraint { return nil }
func (m *mockParabolicTarget) Catalog() string           { return "MOCK" }
func (m *mockParabolicTarget) ID() string                { return "Para" }
func (m *mockParabolicTarget) Name() string              { return "ParaName" }
func (m *mockParabolicTarget) GetDetails(_ *coord.Context, _ ...string) (*TargetDetails, error) {
	return &TargetDetails{}, nil
}

func TestSolveGeometry_GreatestElongation(t *testing.T) {
	t2 := &mockLinearTarget{raRate: 0, startRA: 10, dec: 0}

	t1 := &mockParabolicTarget{
		a: -1.0,
		h: 6.0,
		k: 15.0,
	}

	t1Pos := func(t time.Time) (coord.ICRS, error) {
		hours := float64(t.Sub(time.FromJD(2451545.0, time.UTC)).Hours())
		dec := t1.a*(hours-t1.h)*(hours-t1.h) + t1.k

		return coord.NewICRS(angle.Deg(20), angle.Deg(dec)), nil
	}

	wrapper := &mockDynamicTarget{f: t1Pos}

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
	f func(t time.Time) (coord.ICRS, error)
}

func (m *mockDynamicTarget) Position(t time.Time) (coord.ICRS, error) { return m.f(t) }
func (m *mockDynamicTarget) Constraints() []Constraint                { return nil }
func (m *mockDynamicTarget) Catalog() string                          { return "DYN" }
func (m *mockDynamicTarget) ID() string                               { return "Dyn" }
func (m *mockDynamicTarget) Name() string                             { return "DynName" }
func (m *mockDynamicTarget) GetDetails(_ *coord.Context, _ ...string) (*TargetDetails, error) {
	return &TargetDetails{}, nil
}
