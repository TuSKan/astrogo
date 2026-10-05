package coord_test

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// Work contracts for coord's per-epoch setup: how many times a path evaluates
// the IAU 2006/2000A precession-nutation series, counted through gofaext.
//
// # Why counts
//
// The allocation contracts beside these catch a path that starts allocating.
// They cannot catch one that starts computing something twice, which is what
// NewContext did from May 2026 until #473: Apco13 built the
// celestial-to-intermediate matrix and handed it back, and a second call built
// it again from the same series for the same instant. Every output was
// correct, nothing allocated, and a third of every NewContext went on work
// already done. Only a profile showed it.
//
// A count is the deterministic form of what that profile said, as allocs/op is
// for memory: the same on every machine and under -race, where ns/op is not.
// So it can gate a merge, and a regression of this kind fails the build the
// day it is written rather than waiting for someone to profile.
//
// None of these use t.Parallel: the counter is process-wide, as
// testing.AllocsPerRun's measure is, so a sibling building a Context
// concurrently would be counted here.

// seriesIn returns how many times f evaluates the precession-nutation series.
func seriesIn(f func()) int64 {
	before := gofaext.SeriesEvaluations()

	f()

	return gofaext.SeriesEvaluations() - before
}

// TestNewContextEvaluatesTheSeriesOnce holds NewContext to one evaluation of
// the series, Apco13's, wherever the TT Apco13 derives agrees with t.TT(): from
// 1972 on, leap-second days included. Before 1972 the two TTs differ, so the
// celestial-to-intermediate matrix is built a second time at t.TT(), and the
// contract says so rather than leaving it to be rediscovered; #474 is the
// decision that would bring it to one.
func TestNewContextEvaluatesTheSeriesOnce(t *testing.T) {
	loc, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	atm := atmosphere.AtAltitude(2635)

	for _, c := range []struct {
		name  string
		epoch time.Time
		want  int64
	}{
		{"2023, the allocation contracts' epoch", time.FromJD(2460000.5, time.UTC), 1},
		{"late on a leap-second day", time.Date(2016, time.December, 31, 23, 30, 0, 0, time.LocationUTC), 1},
		{"the first leap-second day", time.Date(1972, time.June, 30, 23, 30, 0, 0, time.LocationUTC), 1},
		{"2100", time.Date(2100, time.January, 1, 0, 0, 0, 0, time.LocationUTC), 1},
		{"1950, where the two TTs differ", time.Date(1950, time.June, 1, 12, 0, 0, 0, time.LocationUTC), 2},
	} {
		if got := seriesIn(func() { coord.NewContext(c.epoch, loc, atm) }); got != c.want {
			t.Errorf("%s: NewContext evaluated the precession-nutation series %d times, want %d.\n"+
				"  Apco13 already returns the celestial-to-intermediate matrix as astrom.Bpn; "+
				"a second evaluation for the same instant is a third of NewContext's cost.",
				c.name, got, c.want)
		}
	}
}

// TestAtTimeEvaluatesNoSeries holds Context.AtTime to what it exists for:
// moving a Context to a nearby instant by updating the Earth rotation alone,
// without touching the series at all.
func TestAtTimeEvaluatesNoSeries(t *testing.T) {
	loc, err := coord.NewGeodetic(angle.Deg(-70.4), angle.Deg(-24.6), 2635)
	if err != nil {
		t.Fatalf("NewGeodetic: %v", err)
	}

	epoch := time.FromJD(2460000.5, time.UTC)
	ctx := coord.NewContext(epoch, loc, atmosphere.AtAltitude(2635))

	if got := seriesIn(func() { ctx.AtTime(epoch.Add(unit.Minutes(1))) }); got != 0 {
		t.Errorf("AtTime evaluated the precession-nutation series %d times, want 0", got)
	}
}
