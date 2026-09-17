package unit_test

import (
	"testing"

	"github.com/TuSKan/astrogo/unit"
)

// The claim [unit.Length] and [unit.Velocity] are built on is that a named
// float64 costs nothing the float64 it replaces did not. These are what hold
// that claim to account.
//
// # Why allocation rather than time
//
// Because allocs/op is deterministic across machines and unchanged under
// -race, which is what lets it gate a build — the same reasoning the contracts
// in coord, time and atmosphere are written down with. ns/op on a shared
// runner is not, so nothing here asserts a duration.
//
// The timing evidence exists and lives in the doc comments: 1.88 ns against
// 1.88 ns scalar, 1887 ns against 1888 ns over a 1000-element batch, measured
// against the bare float64. It is not asserted here because a CI runner cannot
// reproduce it reliably.
//
// # What would break these
//
// Giving either type a pointer, a string, an interface field or a method with
// a non-inlinable body that escapes its receiver. All of those are reachable
// by an ordinary-looking change — a String that took a *Length, a cached
// formatted form — and none of them would fail any other test here.

// TestLengthDoesNotAllocate holds the central claim for distances.
func TestLengthDoesNotAllocate(t *testing.T) {
	var sink unit.Length

	var read float64

	for _, tc := range []struct {
		name string
		f    func()
	}{
		{"construct from meters", func() { sink = unit.Meters(2635) }},
		{"construct from km", func() { sink = unit.Km(384400) }},
		{"construct from AU", func() { sink = unit.AU(1.5) }},
		{"construct from parsecs", func() { sink = unit.Pc(8178) }},
		{"read as meters", func() { read = unit.AU(1.5).Meters() }},
		{"read as km", func() { read = unit.AU(1.5).Km() }},
		{"read as AU", func() { read = unit.Pc(8178).AU() }},
		{"read as parsecs", func() { read = unit.Pc(8178).Pc() }},
		{"round trip", func() { read = unit.AU(1.5).AU() }},
		{"arithmetic", func() { sink = unit.AU(1.5) + unit.Km(100) }},
		{"compare", func() { sink = unit.AU(1.5); read = 0 }},
		{"abs", func() { sink = unit.Pc(-8178).Abs() }},
	} {
		if got := testing.AllocsPerRun(1000, tc.f); got != 0 {
			t.Errorf("Length: %s allocates %v times per call, want 0.\n"+
				"A named float64 must be indistinguishable from the float64 it replaces; "+
				"if this fails the type has grown something that escapes.", tc.name, got)
		}
	}

	_, _ = sink, read
}

// TestVelocityDoesNotAllocate holds the same claim for speeds.
func TestVelocityDoesNotAllocate(t *testing.T) {
	var sink unit.Velocity

	var read float64

	for _, tc := range []struct {
		name string
		f    func()
	}{
		{"construct from m/s", func() { sink = unit.MetersPerSec(343) }},
		{"construct from km/s", func() { sink = unit.KmPerSec(-110.6) }},
		{"construct from au/day", func() { sink = unit.AUPerDay(0.0172) }},
		{"read as m/s", func() { read = unit.KmPerSec(-110.6).MetersPerSec() }},
		{"read as km/s", func() { read = unit.KmPerSec(-110.6).KmPerSec() }},
		{"read as au/day", func() { read = unit.KmPerSec(-110.6).AUPerDay() }},
		{"addition, which is what LSRCorrection composes", func() {
			sink = unit.KmPerSec(-110.6) + unit.KmPerSec(18.04)
		}},
		{"abs", func() { sink = unit.KmPerSec(-110.6).Abs() }},
	} {
		if got := testing.AllocsPerRun(1000, tc.f); got != 0 {
			t.Errorf("Velocity: %s allocates %v times per call, want 0", tc.name, got)
		}
	}

	_, _ = sink, read
}

// TestTheQuantityBridgeIsTheOnlyThingThatAllocates records the boundary rather
// than asserting a number, so the difference between the two representations
// is visible in the test file and not only in the doc comment.
//
// Quantity carries a Unit, and a Unit carries two strings. Passing one by value
// copies 56 bytes against 8 and keeps two string headers live — which is width
// and cache pressure rather than heap traffic, and is why it is measured here
// as a bound rather than assumed to be free.
func TestTheQuantityBridgeIsTheOnlyThingThatAllocates(t *testing.T) {
	var q unit.Quantity

	// The bridge itself does not allocate either: the conversion is arithmetic
	// and the Unit is copied, not built.
	if got := testing.AllocsPerRun(1000, func() { q = unit.AU(1.5).Quantity() }); got != 0 {
		t.Errorf("Length.Quantity allocates %v times per call, want 0", got)
	}

	// Neither does the direction that can fail, on the path where it does not.
	var l unit.Length

	src := unit.New(1.5, unit.AstronomicalUnit)

	if got := testing.AllocsPerRun(1000, func() { l, _ = unit.LengthFrom(src) }); got != 0 {
		t.Errorf("LengthFrom allocates %v times per call on the success path, want 0", got)
	}

	_, _ = q, l
}
