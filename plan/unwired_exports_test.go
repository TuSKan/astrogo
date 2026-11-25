package plan

import (
	"testing"
	"time"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/unit"
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
	solver := NewEventSolver(unit.Hours(6), unit.Seconds(1))

	// One full lunation is 29.53 days, so 40 days contains all four phases
	// with room for the window to start anywhere within a cycle.
	start := epoch.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(unit.Days(40))

	events, err := solver.Find(EventSpec{
		Family: EventFamilyIllumination,
		Kind:   EventAnyPhase,
		Target: moon,
	}, Window{Start: start, End: end})
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
	}, Window{Start: start, End: end})
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

	WithStep(unit.Minutes(5))(&cfg)

	if cfg.step != unit.Minutes(5) {
		t.Errorf("WithStep(5m) set step to %v", cfg.step)
	}

	// It must not disturb the other fields, which is the failure mode of an
	// option written by copying its neighbor.
	if cfg.minAltitude.Degrees() != 0 || cfg.includeMoons || cfg.forceSmallBodyKernels {
		t.Errorf("WithStep changed something other than step: %+v", cfg)
	}

	// Applied after another option, both must survive — options are variadic
	// and order must not matter.
	cfg = visibleTonightConfig{}
	WithPlanetaryMoons()(&cfg)
	WithStep(unit.Seconds(30))(&cfg)

	if !cfg.includeMoons || cfg.step != unit.Seconds(30) {
		t.Errorf("options do not compose: %+v", cfg)
	}
}
