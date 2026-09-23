package iers

import (
	"math"
	"testing"
)

// leapTable is three days of real finals2000A UT1−UTC around the 2016-12-31
// leap second: the two records either side of the step, and one more after it
// so an ordinary interval follows the leap interval.
func leapTable() *Table {
	return &Table{records: []Record{
		{MJD: 57752, DUT1: -0.4073, LOD: 0.0006},
		{MJD: 57753, DUT1: -0.4078, LOD: 0.0006},
		{MJD: 57754, DUT1: +0.5913, LOD: 0.0007}, // 2017-01-01: the leap second is behind it
		{MJD: 57755, DUT1: +0.5907, LOD: 0.0007},
	}}
}

// TestDUT1DoesNotRampAcrossALeapSecond is #377.
//
// Interpolating the raw records, the day ending in the leap second ramped from
// −0.408 s to +0.591 s: half a second wrong at noon, a whole second at
// midnight. UT1 is continuous and UT1−UTC barely moves over a day, so across
// the leap day it must stay within a millisecond of the value it started at.
func TestDUT1DoesNotRampAcrossALeapSecond(t *testing.T) {
	t.Parallel()

	tab := leapTable()

	for _, frac := range []float64{0, 0.25, 0.5, 0.75, 0.9999, 0.99999999} {
		mjd := 57753 + frac

		got, err := tab.EOP(mjd)
		if err != nil {
			t.Fatalf("EOP(%.8f): %v", mjd, err)
		}

		// Between the two records the only honest movement is the rotation
		// drift, -0.5 ms over the day. Anything more is the step leaking in.
		if math.Abs(got.DUT1-(-0.4078)) > 1e-3 {
			t.Errorf("MJD %.8f: DUT1 = %+.4f s, want about -0.408 s — the interpolant is "+
				"crossing the leap second instead of stopping at it", mjd, got.DUT1)
		}
	}
}

// TestDUT1IsExactOnTheFarSideOfTheStep checks the other half of the handling:
// the record on the far side of a leap second is returned as it is, not with
// the step removed.
//
// The adjustment is right for every instant before 0h on 2017-01-01 and wrong
// for 0h itself, which already has the new UTC. Without the exact-match case
// the far-side record would come back a whole second out — the same bug moved
// by one instant.
func TestDUT1IsExactOnTheFarSideOfTheStep(t *testing.T) {
	t.Parallel()

	got, err := leapTable().EOP(57754)
	if err != nil {
		t.Fatalf("EOP: %v", err)
	}

	if got.DUT1 != 0.5913 {
		t.Errorf("DUT1 at 2017-01-01 0h = %+.6f s, want the record's own +0.5913", got.DUT1)
	}
}

// TestDUT1InterpolatesNormallyAwayFromALeap guards the ordinary case: the
// leap handling must not touch an interval that contains no leap second,
// including the one immediately after a leap.
func TestDUT1InterpolatesNormallyAwayFromALeap(t *testing.T) {
	t.Parallel()

	tab := leapTable()

	for _, c := range []struct {
		mjd, want float64
	}{
		// Before the leap: plain interpolation between two same-side records.
		{57752.5, -0.4073 + 0.5*(-0.4078 - -0.4073)},
		// After the leap: plain interpolation on the new side.
		{57754.5, 0.5913 + 0.5*(0.5907-0.5913)},
	} {
		got, err := tab.EOP(c.mjd)
		if err != nil {
			t.Fatalf("EOP(%g): %v", c.mjd, err)
		}

		if math.Abs(got.DUT1-c.want) > 1e-12 {
			t.Errorf("MJD %g: DUT1 = %.10f, want %.10f", c.mjd, got.DUT1, c.want)
		}
	}
}

// TestDUT1HandlesANegativeLeapSecond covers the step that has never happened.
//
// A negative leap second removes a UTC second, so UT1−UTC drops by one instead
// of rising. Nothing about the argument above depends on the sign, and the
// first one is projected for about 2030, so this should not be the day it is
// discovered that it did.
func TestDUT1HandlesANegativeLeapSecond(t *testing.T) {
	t.Parallel()

	tab := &Table{records: []Record{
		{MJD: 62501, DUT1: +0.4012},
		{MJD: 62502, DUT1: -0.5991}, // a second removed at the end of the day before
	}}

	got, err := tab.EOP(62501.75)
	if err != nil {
		t.Fatalf("EOP: %v", err)
	}

	if math.Abs(got.DUT1-0.4) > 1e-3 {
		t.Errorf("DUT1 three-quarters through a day ending in a negative leap second = "+
			"%+.4f s, want about +0.40 s", got.DUT1)
	}
}

// TestLeapJumpThresholdSeparatesTheTwoCases pins why half a second is the line:
// a day of the fastest rotation change on record is far below it, and a leap
// second is far above.
func TestLeapJumpThresholdSeparatesTheTwoCases(t *testing.T) {
	t.Parallel()

	// The largest daily change in UT1−UTC in the modern record is a few
	// milliseconds; 10 ms is a generous ceiling on it.
	const fastestRotationDay = 0.010

	if fastestRotationDay >= leapJumpThreshold {
		t.Errorf("threshold %g s would read a day of rotation as a leap second", leapJumpThreshold)
	}

	if 1-fastestRotationDay <= leapJumpThreshold {
		t.Errorf("threshold %g s would miss a leap second accompanied by a fast rotation day",
			leapJumpThreshold)
	}
}
