package plan

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

func TestBasicTransitionModel(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)

	site, err := NewSite("TestSite", loc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tm := &BasicTransitionModel{
		BaseSetup:           1 * time.Minute,
		SlewRate:            2.0, // degrees per second
		FilterChangePenalty: 30 * time.Second,
	}

	block1 := &Block{
		Target: NewStar("Target 1", 0, 0),
		Config: Configuration{Filter: "V"},
	}

	block2 := &Block{
		// 90 degrees away in Azimuth (approximate test) -> At Zenith, Alt is high, let's just make it a known offset
		Target: NewStar("Target 2", 1.57079632679, 0), // ~90 RA offset
		Config: Configuration{Filter: "R"},
	}

	// FromTime/ToTime intentionally share one value here — this is the
	// common case per Transition.ToTime's doc comment ("approximate, often
	// FromTime").
	now := fixedEpoch()
	ctxAt := movingContext(site.Location(), site.Refraction())

	tr, err := newTransition(nil, block1, now, now, site, ctxAt)
	if err != nil {
		t.Fatalf("newTransition: %v", err)
	}

	// 1. Initial Setup
	overhead, err := tm.Overhead(tr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if overhead != 1*time.Minute {
		t.Errorf("expected 1m setup overhead, got %v", overhead)
	}

	// 2. Filter change + slew
	tr, err = newTransition(block1, block2, now, now, site, ctxAt)
	if err != nil {
		t.Fatalf("newTransition: %v", err)
	}

	// They are placed on the equator, and site is at lat 0. Over 90 deg RA, the great circle
	// or Az difference will be non-zero. Slew should take around 45 seconds (90 deg / 2 deg/s).
	// With 30s filter change, total is ~75s.
	overhead, err = tm.Overhead(tr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if overhead < 30*time.Second {
		t.Errorf("expected overhead to be at least filter penalty (30s), got %v", overhead)
	}
}

// TestBasicTransitionModel_SameEpoch holds the slew priced at one instant,
// the documented common case, to the slew with ToTime five minutes later: the
// two must agree closely, and neither may error or zero out the slew. It began
// as a regression test for Overhead building two Contexts for one instant;
// the strategy now observes both ends, through one moving Context.
func TestBasicTransitionModel_SameEpoch(t *testing.T) {
	loc, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	site, err := NewSite("TestSite", loc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tm := &BasicTransitionModel{SlewRate: 2.0}

	block1 := &Block{Target: NewStar("Target 1", 0, 0)}
	block2 := &Block{Target: NewStar("Target 2", 1.57079632679, 0)}

	now := fixedEpoch()
	later := now.Add(unit.Minutes(5))

	ctxAt := movingContext(site.Location(), site.Refraction())

	sameEpoch, err := newTransition(block1, block2, now, now, site, ctxAt)
	if err != nil {
		t.Fatalf("newTransition (same epoch): %v", err)
	}

	diffEpoch, err := newTransition(block1, block2, now, later, site, ctxAt)
	if err != nil {
		t.Fatalf("newTransition (different epoch): %v", err)
	}

	sameOverhead, err := tm.Overhead(sameEpoch)
	if err != nil {
		t.Fatalf("unexpected error (same epoch): %v", err)
	}

	diffOverhead, err := tm.Overhead(diffEpoch)
	if err != nil {
		t.Fatalf("unexpected error (different epoch): %v", err)
	}

	// Both targets are fixed stars, so over 5 minutes the sky position
	// barely moves — the two overheads should be very close, and neither
	// path should error or silently zero out the slew calculation.
	diff := sameOverhead - diffOverhead
	if diff < 0 {
		diff = -diff
	}

	if diff > 2*time.Second {
		t.Errorf("same-epoch overhead %v diverges too much from different-epoch overhead %v (diff %v)", sameOverhead, diffOverhead, diff)
	}
}
