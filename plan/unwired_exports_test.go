package plan

import (
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// Three exported symbols in this package had no reference anywhere in the
// module — not a caller, not a test, not an example — found by indexing every
// exported declaration against every reference (#106).
//
// A public symbol nothing exercises is not merely uncovered. It is a claim in
// the documentation that nobody has checked, and the repository's own rule is
// that every exported symbol ships with a test. These are the ones that did
// not.

// TestEventAnyPhaseFindsAllFour is the one worth having.
//
// EventAnyPhase is documented as "a wildcard to find all four lunar phases in a
// single pass", and it is reached only through solveIllumination's default
// branch — there is no `case EventAnyPhase` anywhere. Nothing in the module
// passed it, so the claim rested entirely on that default happening to do the
// right thing for a value chosen to fall through to it.
//
// It does. This is what says so.
func TestEventAnyPhaseFindsAllFour(t *testing.T) {
	t.Parallel()

	moon := NewMoon(eph.Default())
	solver := NewEventSolver(6*time.Hour, time.Second)

	// One full lunation is 29.53 days, so 40 days contains all four phases
	// with room for the window to start anywhere within a cycle.
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.LocationUTC)
	end := start.AddDays(40)

	events, err := solver.Find(EventSpec{
		Family: EventFamilyIllumination,
		Kind:   EventAnyPhase,
		Target: moon,
	}, start, end)
	if err != nil {
		t.Fatalf("Find(EventAnyPhase): %v", err)
	}

	seen := make(map[EventKind]int)
	for _, e := range events {
		seen[e.Kind]++
	}

	for _, want := range []EventKind{EventNewMoon, EventFirstQuarter, EventFullMoon, EventLastQuarter} {
		if seen[want] == 0 {
			t.Errorf("EventAnyPhase found no %v across 40 days.\n"+
				"  The wildcard is documented to return all four phases in a single pass, and a "+
				"lunation is 29.53 days, so every one of them falls inside this window.", want)
		}
	}

	// And the contrast that makes it a wildcard rather than a synonym: a named
	// kind must return only itself.
	full, err := solver.Find(EventSpec{
		Family: EventFamilyIllumination,
		Kind:   EventFullMoon,
		Target: moon,
	}, start, end)
	if err != nil {
		t.Fatalf("Find(EventFullMoon): %v", err)
	}

	for _, e := range full {
		if e.Kind != EventFullMoon {
			t.Errorf("asking for EventFullMoon returned a %v; the kind filter is not applied", e.Kind)
		}
	}

	if len(full) >= len(events) {
		t.Errorf("EventFullMoon returned %d events and EventAnyPhase %d.\n"+
			"  The wildcard must find strictly more, or it is not doing anything.",
			len(full), len(events))
	}

	// Events come back sorted by time, which a caller merging four phase
	// searches depends on.
	for i := 1; i < len(events); i++ {
		if events[i].Time.Before(events[i-1].Time) {
			t.Fatalf("event %d is before event %d; the combined pass is not sorted", i, i-1)
		}
	}
}

// TestNewEarthTargetsEarth covers the one planet constructor nothing used.
//
// Earth is the odd member of the set: every other NewX names something to point
// a telescope at. This one is mostly a way to reach Earth's own state through
// Provider(), and whether it earns its place in the public API is a question
// for #106 rather than for a test. What a test can settle is that it targets
// the body it names, which is the way a constructor written by copying its
// neighbour goes wrong.
func TestNewEarthTargetsEarth(t *testing.T) {
	t.Parallel()

	earth := NewEarth(eph.Default())

	if earth == nil {
		t.Fatal("NewEarth returned nil")
	}

	if earth.Name() != "Earth" {
		t.Errorf("Name() = %q, want Earth", earth.Name())
	}

	if got := earth.EphID(); got != eph.Earth {
		t.Errorf("EphID() = %v, want eph.Earth (%v) — this constructor targets the wrong body",
			got, eph.Earth)
	}

	// And the id is actually used rather than merely stored: two constructors
	// must not produce the same sky position.
	epoch := time.Date(2026, 3, 20, 12, 0, 0, 0, time.LocationUTC)

	here, err := earth.Position(epoch)
	if err != nil {
		t.Fatalf("Position: %v", err)
	}

	there, err := NewMars(eph.Default()).Position(epoch)
	if err != nil {
		t.Fatalf("Mars Position: %v", err)
	}

	if here.RA() == there.RA() && here.Dec() == there.Dec() {
		t.Error("NewEarth and NewMars produce the same position; the ephemeris id is not reaching the lookup")
	}

	// Not asserted: Dist(). Position returns a direction and leaves the
	// distance at zero for every body -- eph.ToICRS computes the radius and
	// does not carry it -- so a check on it would pass whatever this
	// constructor did. TargetDetails.Distance is the populated one (measured:
	// Mars 2.31 AU), and it goes through a different path.
}

// TestWithStepSetsTheSamplingCadence covers the VisibleTonight option that had
// no caller.
//
// WithMinAltitude, WithPlanetaryMoons and WithSmallBodyKernels are all
// exercised; WithStep was the one that was not, so nothing checked that the
// option a caller reaches for to trade accuracy against time actually reaches
// the window search.
func TestWithStepSetsTheSamplingCadence(t *testing.T) {
	t.Parallel()

	var cfg visibleTonightConfig

	if cfg.step != 0 {
		t.Fatalf("precondition: a zero config has step %v, want 0", cfg.step)
	}

	WithStep(5 * time.Minute)(&cfg)

	if cfg.step != 5*time.Minute {
		t.Errorf("WithStep(5m) set step to %v", cfg.step)
	}

	// It must not disturb the other fields, which is the failure mode of an
	// option written by copying its neighbour.
	if cfg.minAltitude.Degrees() != 0 || cfg.includeMoons || cfg.forceSmallBodyKernels {
		t.Errorf("WithStep changed something other than step: %+v", cfg)
	}

	// Applied after another option, both must survive — options are variadic
	// and order must not matter.
	cfg = visibleTonightConfig{}
	WithPlanetaryMoons()(&cfg)
	WithStep(30 * time.Second)(&cfg)

	if !cfg.includeMoons || cfg.step != 30*time.Second {
		t.Errorf("options do not compose: %+v", cfg)
	}
}
