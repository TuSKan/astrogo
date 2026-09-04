package time_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/time"
)

// None of the tests in this file call t.Parallel, and none may: they mutate
// the process-wide ΔAT table, and time's own scale tests read it. Go runs the
// sequential tests in a package to completion before resuming the parallel
// ones, so a deferred ResetLeapSeconds here lands before those wake up.

// publishedRecord is the ΔAT table as IERS publishes it, in the registry's
// own form.
//
// Deliberately a second copy of leapSecondRecord rather than a shared
// fixture. That table is a golden record checked against the IERS Data Center
// entry by entry, and a test that feeds it into the registry and reads it back
// would be comparing a value against itself. Here the entries are inputs, and
// what is asserted is what the *registry* does with them.
var publishedRecord = []time.LeapSecond{
	{Year: 1972, Month: 1, Day: 1, DeltaAT: 10}, {Year: 1972, Month: 7, Day: 1, DeltaAT: 11},
	{Year: 1973, Month: 1, Day: 1, DeltaAT: 12}, {Year: 1974, Month: 1, Day: 1, DeltaAT: 13},
	{Year: 1975, Month: 1, Day: 1, DeltaAT: 14}, {Year: 1976, Month: 1, Day: 1, DeltaAT: 15},
	{Year: 1977, Month: 1, Day: 1, DeltaAT: 16}, {Year: 1978, Month: 1, Day: 1, DeltaAT: 17},
	{Year: 1979, Month: 1, Day: 1, DeltaAT: 18}, {Year: 1980, Month: 1, Day: 1, DeltaAT: 19},
	{Year: 1981, Month: 7, Day: 1, DeltaAT: 20}, {Year: 1982, Month: 7, Day: 1, DeltaAT: 21},
	{Year: 1983, Month: 7, Day: 1, DeltaAT: 22}, {Year: 1985, Month: 7, Day: 1, DeltaAT: 23},
	{Year: 1988, Month: 1, Day: 1, DeltaAT: 24}, {Year: 1990, Month: 1, Day: 1, DeltaAT: 25},
	{Year: 1991, Month: 1, Day: 1, DeltaAT: 26}, {Year: 1992, Month: 7, Day: 1, DeltaAT: 27},
	{Year: 1993, Month: 7, Day: 1, DeltaAT: 28}, {Year: 1994, Month: 7, Day: 1, DeltaAT: 29},
	{Year: 1996, Month: 1, Day: 1, DeltaAT: 30}, {Year: 1997, Month: 7, Day: 1, DeltaAT: 31},
	{Year: 1999, Month: 1, Day: 1, DeltaAT: 32}, {Year: 2006, Month: 1, Day: 1, DeltaAT: 33},
	{Year: 2009, Month: 1, Day: 1, DeltaAT: 34}, {Year: 2012, Month: 7, Day: 1, DeltaAT: 35},
	{Year: 2015, Month: 7, Day: 1, DeltaAT: 36}, {Year: 2017, Month: 1, Day: 1, DeltaAT: 37},
}

// extended returns the published record with extra entries appended.
func extended(extra ...time.LeapSecond) []time.LeapSecond {
	out := make([]time.LeapSecond, 0, len(publishedRecord)+len(extra))
	out = append(out, publishedRecord...)

	return append(out, extra...)
}

// taiMinusUTC measures ΔAT as the conversion actually applies it, in seconds.
//
// Reading it out of Time rather than calling the unexported lookup is the
// point: the registry is only worth anything if it reaches the scale
// conversions, and a test against the lookup alone would pass with all four
// call sites unwired.
func taiMinusUTC(t *testing.T, when time.Time) float64 {
	t.Helper()

	u1, u2 := when.UTC().JDParts()
	a1, a2 := when.TAI().JDParts()

	return ((a1 - u1) + (a2 - u2)) * 86400.0
}

// ttMinusUTC is the same measurement on the TT path, which reaches ΔAT by a
// different branch.
func ttMinusUTC(t *testing.T, when time.Time) float64 {
	t.Helper()

	u1, u2 := when.UTC().JDParts()
	t1, t2 := when.TT().JDParts()

	return ((t1 - u1) + (t2 - u2)) * 86400.0
}

func utc(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 12, 0, 0, 0, time.LocationUTC)
}

// TestRegisterLeapSecondsCarriesAnAnnouncementTheBuiltinTableLacks is the
// whole reason the registry exists.
//
// # The gap it closes
//
// gofa's table ends at 2017-01-01 (37 s) and is compiled into its source, so a
// newly announced leap second reaches astrogo only through a go.mod bump and a
// release. Measured here: gofa answers 37 s for 2027 with status 0 — not
// "dubious year", just confidently wrong about a date past its table. Nothing
// in the return value distinguishes "37 because the record says so" from
// "37 because this table stops at 2017".
//
// So the assertion is not that some number changed. It is that a date after
// the built-in table's end takes the registered value while every date before
// it does not move at all — an extension, not a replacement.
func TestRegisterLeapSecondsCarriesAnAnnouncementTheBuiltinTableLacks(t *testing.T) {
	defer time.ResetLeapSeconds()

	// A hypothetical positive leap second at the end of 2026. Whether one is
	// ever announced is beside the point; what is pinned is that an announced
	// one would be carried.
	const announced = 38

	future := utc(2027, time.June, 1)
	past := utc(2016, time.June, 1)

	if got := taiMinusUTC(t, future); got != 37 {
		t.Fatalf("precondition: built-in table gives ΔAT = %g at 2027, want 37", got)
	}

	if err := time.RegisterLeapSeconds(
		extended(time.LeapSecond{Year: 2027, Month: 1, Day: 1, DeltaAT: announced}),
		"bulletin-c",
	); err != nil {
		t.Fatalf("registering an extension of the record: %v", err)
	}

	if got := taiMinusUTC(t, future); got != announced {
		t.Errorf("TAI−UTC at 2027-06-01 = %g s, want %d s.\n"+
			"  The registered table carries an entry the built-in one does not, and "+
			"Time.TAI did not use it.", got, announced)
	}

	// The same value has to reach TT, which gets there by its own branch.
	if got, want := ttMinusUTC(t, future), float64(announced)+32.184; math.Abs(got-want) > 1e-6 {
		t.Errorf("TT−UTC at 2027-06-01 = %g s, want %g s.\n"+
			"  ΔAT reached the TAI conversion but not the TT one.", got, want)
	}

	// And history does not move. This is the property that makes registering
	// late safe: an epoch before the built-in table's end is unaffected by
	// whether registration has happened yet.
	if got := taiMinusUTC(t, past); got != 36 {
		t.Errorf("TAI−UTC at 2016-06-01 = %g s, want 36 s.\n"+
			"  Registering an extension changed a value the built-in table already "+
			"knew — it is behaving as a replacement, not a superset.", got)
	}

	if got := time.LeapSecondSource(); got != "bulletin-c" {
		t.Errorf("LeapSecondSource() = %q, want %q", got, "bulletin-c")
	}
}

// TestUTCFromTAIInvertsAcrossARegisteredEntry pins the reverse conversion,
// which reads ΔAT twice — once on a TAI-shaped guess, then again at the
// resulting UTC in case the guess straddled a step.
//
// A registry wired into the forward direction only would pass every test
// above and still make TAI→UTC→TAI lose a second.
func TestUTCFromTAIInvertsAcrossARegisteredEntry(t *testing.T) {
	defer time.ResetLeapSeconds()

	if err := time.RegisterLeapSeconds(
		extended(time.LeapSecond{Year: 2027, Month: 1, Day: 1, DeltaAT: 38}),
		"bulletin-c",
	); err != nil {
		t.Fatalf("register: %v", err)
	}

	for _, when := range []time.Time{
		utc(2016, time.June, 1), // before the built-in table's end
		utc(2020, time.June, 1), // after it, before the registered entry
		utc(2027, time.June, 1), // after the registered entry
		utc(2026, time.December, 31),
		utc(2027, time.January, 1),
	} {
		got := when.TAI().UTC()

		g1, g2 := got.JDParts()
		w1, w2 := when.UTC().JDParts()

		if d := math.Abs(((g1 - w1) + (g2 - w2)) * 86400.0); d > 1e-6 {
			t.Errorf("%s: UTC→TAI→UTC lost %g s", when.Format(time.RFC3339), d)
		}
	}
}

// TestRegisterLeapSecondsRefusesToRestateTheRecord is the guard that makes the
// whole mechanism safe to use.
//
// A registration may add entries the built-in record does not have. It may not
// give a different answer for one it does: that means one of the two sources
// is stale or corrupt, and quietly preferring the newcomer would move every
// epoch after that date by a second with nothing said.
//
// The rejection must also change nothing — a half-applied registration is
// worse than a refused one.
func TestRegisterLeapSecondsRefusesToRestateTheRecord(t *testing.T) {
	defer time.ResetLeapSeconds()

	// Every value one second low: the shape a parser produces when it drops
	// the table's first row and keeps the rest. Ascending, every step exactly
	// +1 — so the well-formedness check passes it, and only a comparison
	// against the built-in record catches it. That is the case this guard is
	// for; a table that is merely malformed would have been rejected already.
	bad := extended()
	for i := range bad {
		bad[i].DeltaAT--
	}

	err := time.RegisterLeapSeconds(bad, "corrupt")
	if !errors.Is(err, time.ErrLeapSecondConflict) {
		t.Fatalf("registering a table that contradicts the built-in record returned %v, "+
			"want ErrLeapSecondConflict", err)
	}

	if got := time.LeapSecondSource(); got != "builtin" {
		t.Errorf("after a rejected registration LeapSecondSource() = %q, want %q — "+
			"the table was installed anyway", got, "builtin")
	}

	if got := taiMinusUTC(t, utc(2020, time.June, 1)); got != 37 {
		t.Errorf("after a rejected registration TAI−UTC = %g s, want 37 s", got)
	}
}

// TestRegisterLeapSecondsAcceptsANegativeLeapSecond pins that the validator
// permits the one kind of entry that has never yet occurred.
//
// Every leap second so far has been positive, and Earth's rotation has been
// speeding up: a negative one — skipping 23:59:59 rather than inserting
// 23:59:60 — is now considered plausible before leap seconds end in 2035
// (Agnew 2024; Malkin 2024). The ITU-R has permitted it since 1972.
//
// A validator written around "each entry is one more than the last" would
// reject the first real one, at the worst possible moment.
func TestRegisterLeapSecondsAcceptsANegativeLeapSecond(t *testing.T) {
	defer time.ResetLeapSeconds()

	if err := time.RegisterLeapSeconds(
		extended(time.LeapSecond{Year: 2029, Month: 7, Day: 1, DeltaAT: 36}),
		"bulletin-c",
	); err != nil {
		t.Fatalf("a negative leap second was rejected: %v", err)
	}

	if got := taiMinusUTC(t, utc(2029, time.December, 1)); got != 36 {
		t.Errorf("TAI−UTC after a negative leap second = %g s, want 36 s", got)
	}
}

// TestRegisterLeapSecondsRejectsAMalformedTable covers the shapes that are not
// a record at all, each of which would otherwise be installed and then read
// back as a plausible number.
func TestRegisterLeapSecondsRejectsAMalformedTable(t *testing.T) {
	defer time.ResetLeapSeconds()

	cases := map[string][]time.LeapSecond{
		"empty": {},
		"out of order": {
			{Year: 1972, Month: 1, Day: 1, DeltaAT: 10},
			{Year: 1973, Month: 1, Day: 1, DeltaAT: 12},
			{Year: 1972, Month: 7, Day: 1, DeltaAT: 11},
		},
		"an entry skipped, so a two-second step": {
			{Year: 1972, Month: 1, Day: 1, DeltaAT: 10},
			{Year: 1973, Month: 1, Day: 1, DeltaAT: 12},
		},
		"duplicate date": {
			{Year: 1972, Month: 1, Day: 1, DeltaAT: 10},
			{Year: 1972, Month: 1, Day: 1, DeltaAT: 11},
		},
	}

	for name, table := range cases {
		t.Run(name, func(t *testing.T) {
			if err := time.RegisterLeapSeconds(table, "malformed"); !errors.Is(err, time.ErrLeapSecondOrder) {
				t.Errorf("returned %v, want ErrLeapSecondOrder", err)
			}

			if got := time.LeapSecondSource(); got != "builtin" {
				t.Errorf("LeapSecondSource() = %q after a rejected table, want %q", got, "builtin")
			}
		})
	}
}

// TestResetLeapSecondsRestoresTheBuiltinTable pins the escape hatch every
// other test in this file depends on for isolation.
func TestResetLeapSecondsRestoresTheBuiltinTable(t *testing.T) {
	defer time.ResetLeapSeconds()

	if got := time.LeapSecondSource(); got != "builtin" {
		t.Fatalf("precondition: LeapSecondSource() = %q, want %q", got, "builtin")
	}

	if err := time.RegisterLeapSeconds(
		extended(time.LeapSecond{Year: 2027, Month: 1, Day: 1, DeltaAT: 38}),
		"bulletin-c",
	); err != nil {
		t.Fatalf("register: %v", err)
	}

	time.ResetLeapSeconds()

	if got := time.LeapSecondSource(); got != "builtin" {
		t.Errorf("LeapSecondSource() = %q after reset, want %q", got, "builtin")
	}

	if got := taiMinusUTC(t, utc(2027, time.June, 1)); got != 37 {
		t.Errorf("TAI−UTC at 2027 = %g s after reset, want the built-in 37 s", got)
	}
}

// TestEpochsBeforeARegisteredTableBeginsFallBackToTheBuiltinOne pins the
// behaviour a truncated table gets.
//
// A source need not start at 1972 — a Bulletin C extract might carry only
// recent entries. Below its first entry it has no opinion, and the built-in
// table answers rather than the caller silently getting the table's first
// value applied to all of history, or zero.
//
// Mixing the two is safe precisely because registration is superset-only:
// anything the registered table does say below the built-in record's end was
// checked to agree with it, so the two cannot disagree in the region where
// this falls through.
func TestEpochsBeforeARegisteredTableBeginsFallBackToTheBuiltinOne(t *testing.T) {
	defer time.ResetLeapSeconds()

	// Starts at 1990, so it says nothing about the 1970s or 80s.
	truncated := publishedRecord[15:]
	if truncated[0].Year != 1990 {
		t.Fatalf("fixture drifted: truncated table starts at %d, want 1990", truncated[0].Year)
	}

	if err := time.RegisterLeapSeconds(truncated, "partial"); err != nil {
		t.Fatalf("register: %v", err)
	}

	for _, tc := range []struct {
		when time.Time
		want float64
	}{
		{utc(1975, time.June, 1), 14}, // before the table: built-in answers
		{utc(1988, time.June, 1), 24}, // still before it
		{utc(1990, time.June, 1), 25}, // its first entry
		{utc(2000, time.June, 1), 32}, // well inside it
	} {
		if got := taiMinusUTC(t, tc.when); got != tc.want {
			t.Errorf("%s: TAI−UTC = %g s, want %g s",
				tc.when.Format(time.RFC3339), got, tc.want)
		}
	}
}

// TestPre1972EpochsIgnoreTheRegistryEntirely pins that registering a table
// cannot reach into the era where ΔAT does not exist.
//
// Before 1972 UTC was not offset from TAI by whole seconds at all — it ran at
// a rubber rate with fractional offsets — so the conversion uses the Espenak
// & Meeus ΔT polynomials instead. A registry that leaked into that branch
// would replace a 40-second-scale historical offset with a 10-second one and
// still look ordinary.
func TestPre1972EpochsIgnoreTheRegistryEntirely(t *testing.T) {
	defer time.ResetLeapSeconds()

	when := utc(1900, time.June, 1)

	before := ttMinusUTC(t, when)

	if err := time.RegisterLeapSeconds(
		extended(time.LeapSecond{Year: 2027, Month: 1, Day: 1, DeltaAT: 38}),
		"bulletin-c",
	); err != nil {
		t.Fatalf("register: %v", err)
	}

	if after := ttMinusUTC(t, when); after != before {
		t.Errorf("TT−UTC at 1900 moved from %g s to %g s when a leap-second table "+
			"was registered.\n  Pre-1972 epochs use ΔT, not ΔAT, and must not be "+
			"reachable from the registry.", before, after)
	}

	// And the value is the ΔT polynomial's, not a leap-second count. At 1900
	// ΔT is about −2.8 s, while the smallest ΔAT-based answer the registry
	// could produce is 10 + 32.184 s — so the two are not near each other and
	// this cannot pass by coincidence.
	if want := time.DeltaT(when.DecimalYear()); math.Abs(before-want) > 1e-9 {
		t.Errorf("TT−UTC at 1900 = %g s, want ΔT = %g s", before, want)
	}
}
