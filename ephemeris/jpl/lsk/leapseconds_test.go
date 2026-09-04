package lsk_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/jpl/lsk"
	"github.com/TuSKan/astrogo/time"
)

// taiMinusUTCSeconds measures ΔAT as the scale conversion actually applies it.
//
// Not an exact comparison: the two Julian Dates are near 2.46e6, where one ULP
// is about 40 µs, so a whole-second value comes back a picosecond or so off.
// The quantity under test is a leap second, and 1 µs is eleven orders of
// magnitude below it.
func taiMinusUTCSeconds(when time.Time) float64 {
	u1, u2 := when.UTC().JDParts()
	a1, a2 := when.TAI().JDParts()

	return ((a1 - u1) + (a2 - u2)) * 86400.0
}

// TestRegisterLeapSecondsRejectsAnEmptyTable pins that a Reader carrying no
// DELTA_AT block is refused, and — the part that matters — that the refusal
// leaves whatever was in force untouched rather than clearing it.
//
// The assertion is "unchanged", not "builtin". Registration is process-wide,
// so any earlier test in this package that built a provider has already put
// the kernel in force; asserting a literal source here would make this test
// depend on which tests ran before it, which is exactly the kind of coupling
// that later gets "fixed" by loosening the assertion.
func TestRegisterLeapSecondsRejectsAnEmptyTable(t *testing.T) {
	before := time.LeapSecondSource()

	if err := lsk.RegisterLeapSeconds(&lsk.Reader{}); err == nil {
		t.Error("an empty DELTA_AT table was accepted")
	}

	if got := time.LeapSecondSource(); got != before {
		t.Errorf("a rejected registration changed the source from %q to %q",
			before, got)
	}
}

// TestProviderRegistersItsKernelsLeapSeconds pins the wiring that makes the
// registry reach real callers.
//
// A provider drives its epochs from the kernel's DELTA_AT and DELTA_T_A blocks
// — that is what UTCToET does. If time's UTC↔TAI↔TT conversions kept following
// a table pinned by go.mod while the SPK was evaluated at an ET derived from
// the kernel, the two would disagree by a second the moment a new leap second
// was announced, in opposite directions, with nothing to say so.
//
// openKernel builds a provider, so by the time it returns the registration has
// already happened; that is exactly what this asserts.
func TestProviderRegistersItsKernelsLeapSeconds(t *testing.T) {
	defer time.ResetLeapSeconds()

	_ = openKernel(t)

	if got := time.LeapSecondSource(); got != "kernel" {
		t.Errorf("after jpl.NewProvider, LeapSecondSource() = %q, want %q.\n"+
			"  The provider computes ET from the kernel's own tables, so time must "+
			"be using the same kernel's leap seconds.", got, "kernel")
	}
}

// TestRealKernelTableMatchesTheBuiltinRecord is the check that the conversion
// is right about a file nobody here wrote.
//
// naif0012.tls carries 28 DELTA_AT entries ending at 2017-01-01 (37 s) —
// exactly where gofa's compiled table ends. So registering it must be accepted
// (it agrees everywhere the two overlap, which is everywhere) and must leave
// every conversion numerically unchanged.
//
// That makes this a strong test of the *plumbing* and a weak one of the data:
// gofa's table, NAIF's kernel and the discontinuities in IERS finals2000A are
// three republications of one IERS source, so their agreement is a consistency
// check rather than validation — see time's leapSecondRecord doc comment and
// metrology.Reference.SharedAncestor. What it does prove is that a real
// kernel's block survives parsing, JD-to-calendar conversion and the superset
// check intact, and that is the part that could silently break.
func TestRealKernelTableMatchesTheBuiltinRecord(t *testing.T) {
	defer time.ResetLeapSeconds()

	r := openKernel(t)

	// openKernel builds a provider, which registers already. Undo that so the
	// baseline below is genuinely the built-in table — otherwise the
	// before/after comparison compares the kernel against itself.
	time.ResetLeapSeconds()

	probes := []struct {
		when time.Time
		want float64
	}{
		{time.Date(1972, time.January, 1, 0, 0, 1, 0, time.LocationUTC), 10},
		{time.Date(1972, time.June, 30, 12, 0, 0, 0, time.LocationUTC), 10},
		{time.Date(1972, time.July, 1, 0, 0, 1, 0, time.LocationUTC), 11},
		{time.Date(1985, time.July, 1, 0, 0, 1, 0, time.LocationUTC), 23},
		{time.Date(1999, time.January, 1, 0, 0, 1, 0, time.LocationUTC), 32},
		{time.Date(2015, time.July, 1, 0, 0, 1, 0, time.LocationUTC), 36},
		{time.Date(2016, time.December, 31, 12, 0, 0, 0, time.LocationUTC), 36},
		{time.Date(2017, time.January, 1, 0, 0, 1, 0, time.LocationUTC), 37},
		{time.Date(2026, time.June, 1, 12, 0, 0, 0, time.LocationUTC), 37},
	}

	const tolerance = 1e-6 // seconds; see taiMinusUTCSeconds

	before := make([]float64, len(probes))
	for i, p := range probes {
		before[i] = taiMinusUTCSeconds(p.when)
	}

	if err := lsk.RegisterLeapSeconds(r); err != nil {
		t.Fatalf("registering naif0012.tls was rejected: %v\n"+
			"  The kernel's DELTA_AT block and gofa's compiled table are the same "+
			"IERS record, so a conflict here means the JD-to-calendar conversion is "+
			"shifting entries.", err)
	}

	if got := time.LeapSecondSource(); got != "kernel" {
		t.Fatalf("LeapSecondSource() = %q, want %q", got, "kernel")
	}

	for i, p := range probes {
		got := taiMinusUTCSeconds(p.when)

		if math.Abs(got-p.want) > tolerance {
			t.Errorf("%s: TAI−UTC = %g s from the kernel, want %g s",
				p.when.Format(time.RFC3339), got, p.want)
		}

		if math.Abs(got-before[i]) > tolerance {
			t.Errorf("%s: registering the kernel changed TAI−UTC from %g s to %g s.\n"+
				"  naif0012.tls ends where gofa's table ends, so every value here must "+
				"be identical either way.", p.when.Format(time.RFC3339), before[i], got)
		}
	}
}
