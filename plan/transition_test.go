package plan

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
)

func TestBasicTransitionModel(t *testing.T) {
	loc, _ := coord.NewGeodetic(angle.Zero(), angle.Zero(), 0)
	site, err := NewSite("TestSite", loc, angle.Zero(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tm := &BasicTransitionModel{
		BaseSetup:           1 * time.Minute,
		SlewRate:            2.0, // degrees per second
		FilterChangePenalty: 30 * time.Second,
	}

	block1 := &Block{
		Target: NewTarget(catalog.Target{Name: "Target 1", Coord: coord.NewICRS(0, 0)}, nil),
		Config: Configuration{Filter: "V"},
	}

	block2 := &Block{
		// 90 degrees away in Azimuth (approximate test) -> At Zenith, Alt is high, let's just make it a known offset
		Target: NewTarget(catalog.Target{Name: "Target 2", Coord: coord.NewICRS(1.57079632679, 0)}, nil), // ~90 RA offset
		Config: Configuration{Filter: "R"},
	}

	ctx := TransitionContext{
		FromBlock: nil,
		ToBlock:   block1,
		FromTime:  time.NowUTC(),
		ToTime:    time.NowUTC(),
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
