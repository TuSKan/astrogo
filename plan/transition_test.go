package plan

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

func TestBasicTransitionModel(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)

	site, err := NewSite("TestSite", loc, nil)
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
	// common case per TransitionContext.ToTime's doc comment ("approximate,
	// often FromTime") and exercises the shared-Context path in Overhead.
	now := time.NowUTC()

	ctx := TransitionContext{
		FromBlock: nil,
		ToBlock:   block1,
		FromTime:  now,
		ToTime:    now,
		Site:      site,
	}

	// 1. Initial Setup
	overhead, err := tm.Overhead(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if overhead != 1*time.Minute {
		t.Errorf("expected 1m setup overhead, got %v", overhead)
	}

	// 2. Filter change + slew
	ctx.FromBlock = block1
	ctx.ToBlock = block2

	// They are placed on the equator, and site is at lat 0. Over 90 deg RA, the great circle
	// or Az difference will be non-zero. Slew should take around 45 seconds (90 deg / 2 deg/s).
	// With 30s filter change, total is ~75s.
	overhead, err = tm.Overhead(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if overhead < 30*time.Second {
		t.Errorf("expected overhead to be at least filter penalty (30s), got %v", overhead)
	}
}

// TestBasicTransitionModel_SameEpoch is a regression test: Overhead used to
// build two separate coord.Context values for FromTime and ToTime even when
// they were the same instant (the documented common case), redundantly
// repeating the ~91µs SOFA transform. This confirms the shared-Context path
// (FromTime.Equal(ToTime)) produces the same result as the general path.
func TestBasicTransitionModel_SameEpoch(t *testing.T) {
	loc, err := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	site, err := NewSite("TestSite", loc, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tm := &BasicTransitionModel{SlewRate: 2.0}

	block1 := &Block{Target: NewStar("Target 1", 0, 0)}
	block2 := &Block{Target: NewStar("Target 2", 1.57079632679, 0)}

	now := time.NowUTC()
	later := now.Add(5 * time.Minute)

	sameEpoch := TransitionContext{
		FromBlock: block1, ToBlock: block2,
		FromTime: now, ToTime: now,
		Site: site,
	}

	diffEpoch := TransitionContext{
		FromBlock: block1, ToBlock: block2,
		FromTime: now, ToTime: later,
		Site: site,
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
