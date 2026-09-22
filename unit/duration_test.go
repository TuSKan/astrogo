package unit_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestDurationRoundTripsThroughEveryUnit checks each constructor against its
// own accessor.
//
// Worth doing per unit rather than once: the constructors read their scale
// factors from the package's [unit.Unit] table, so a mistyped entry there —
// an hour of 360 seconds, a day of 84600 — would be self-consistent within one
// pair and wrong against every other.
func TestDurationRoundTripsThroughEveryUnit(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		make func(float64) unit.Duration
		read func(unit.Duration) float64
		// seconds in one of this unit, stated independently of the table.
		perUnit float64
	}{
		{"seconds", unit.Seconds, unit.Duration.Seconds, 1},
		{"minutes", unit.Minutes, unit.Duration.Minutes, 60},
		{"hours", unit.Hours, unit.Duration.Hours, 3600},
		{"days", unit.Days, unit.Duration.Days, 86400},
		{"Julian years", unit.JulianYears, unit.Duration.JulianYears, 365.25 * 86400},
	} {
		d := tc.make(3)

		if got := tc.read(d); math.Abs(got-3) > 1e-12 {
			t.Errorf("%s: 3 out and back gave %v", tc.name, got)
		}

		if got := d.Seconds(); math.Abs(got-3*tc.perUnit) > 1e-9 {
			t.Errorf("%s: 3 is %v s, want %v", tc.name, got, 3*tc.perUnit)
		}
	}
}

// TestDurationHoldsAnAstronomicalInterval is the reason this type exists
// rather than the standard library's.
//
// time.Duration is an int64 nanosecond count and saturates just past ±292
// years. Every interval below is one astrogo actually works with and that type
// cannot hold, so the comparison is not hypothetical.
func TestDurationHoldsAnAstronomicalInterval(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		years float64
	}{
		{"the span of a JPL DE441 kernel, roughly", 30_000},
		{"one precession cycle", 25_772},
		{"J2000 back to JD 0", 6713},
		{"the age of the Earth", 4.54e9},
	} {
		d := unit.JulianYears(tc.years)

		if got := d.JulianYears(); math.Abs(got-tc.years)/tc.years > 1e-12 {
			t.Errorf("%s: %v years came back as %v", tc.name, tc.years, got)
		}

		// And the point: the standard library's type does not survive it.
		if ns := tc.years * 365.25 * 86400 * 1e9; ns <= math.MaxInt64 {
			t.Errorf("%s: %v years fits in an int64 nanosecond count, so it is "+
				"not evidence for this type", tc.name, tc.years)
		}
	}
}

// TestDurationIsNotTheStandardLibrarysType guards the trap this type adds to
// the ones [unit.Length] already documents.
//
// Both are named numeric types, so an untyped constant converts silently into
// either — and means different things. Nothing can stop that at compile time;
// what this test does is record the factor, so a reader who hits a
// billion-fold discrepancy finds the reason written down.
func TestDurationIsNotTheStandardLibrarysType(t *testing.T) {
	t.Parallel()

	const five = 5

	var (
		astro unit.Duration = five
		std   time.Duration = five
	)

	if astro.Seconds() != 5 {
		t.Errorf("a bare 5 is %v here, want 5 seconds", astro.Seconds())
	}

	if std.Seconds() != 5e-9 {
		t.Errorf("a bare 5 is %v to the standard library, want 5 nanoseconds", std.Seconds())
	}

	// A billion apart, from the same literal. Write unit.Seconds(5).
	if ratio := astro.Seconds() / std.Seconds(); ratio != 1e9 {
		t.Errorf("the two readings of 5 differ by %v, want 1e9", ratio)
	}
}

// TestDurationQuantityRoundTrip covers the bridge to the dimensional
// machinery, in both directions and in the failing direction.
func TestDurationQuantityRoundTrip(t *testing.T) {
	t.Parallel()

	d := unit.Hours(1.5)

	back, err := unit.DurationFrom(d.Quantity())
	if err != nil {
		t.Fatalf("DurationFrom: %v", err)
	}

	if back != d {
		t.Errorf("round trip gave %v, want %v", back, d)
	}

	// A length is not a duration, and this is where that is noticed.
	if _, err := unit.DurationFrom(unit.Km(1).Quantity()); err == nil {
		t.Error("DurationFrom accepted a length")
	}
}

// TestDurationStringPicksTheUnitAReaderExpects pins the rendering thresholds,
// including both sides of each one.
func TestDurationStringPicksTheUnitAReaderExpects(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		d    unit.Duration
		want string
	}{
		{unit.Seconds(30), "30 s"},
		{unit.Seconds(59.9), "59.9 s"},
		{unit.Minutes(1), "1 min"},
		{unit.Minutes(45), "45 min"},
		{unit.Minutes(91.6), "1.52667 h"}, // an ISS orbit is past the hour threshold
		{unit.Hours(1), "1 h"},
		{unit.Hours(23.9), "23.9 h"},
		{unit.Days(1), "1 d"},
		{unit.Days(365), "365 d"},
		{unit.JulianYears(1), "1 a"},
		{unit.JulianYears(248), "248 a"}, // Pluto
		{unit.Seconds(-30), "-30 s"},
		{0, "0 s"},
	} {
		if got := tc.d.String(); got != tc.want {
			t.Errorf("String() = %q, want %q", got, tc.want)
		}
	}
}

// TestDurationAbsAndIsZero covers the two small predicates.
func TestDurationAbsAndIsZero(t *testing.T) {
	t.Parallel()

	if got := unit.Days(-2).Abs(); got != unit.Days(2) {
		t.Errorf("Abs of -2 days = %v, want 2 days", got)
	}

	if !unit.Seconds(0).IsZero() {
		t.Error("zero is not IsZero")
	}

	if unit.Seconds(math.SmallestNonzeroFloat64).IsZero() {
		t.Error("the smallest nonzero float reported IsZero")
	}
}
