package time_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/gofaext"
)

// leapStep is one entry of the published ΔAT (TAI−UTC) record.
type leapStep struct {
	year, month, day int
	deltaAT          float64
}

// leapSecondRecord is the complete published leap-second history: every step
// IERS has announced since UTC began accumulating whole seconds on 1972-01-01.
//
// # Why this is pinned rather than computed
//
// Almost everything else this library asserts is a *measurement* with a
// tolerance. This is not. ΔAT is a finite, immutable administrative record —
// 28 entries, each fixed by an IERS Bulletin C announcement, none of which can
// change retroactively. The table only ever grows, and only at its end.
//
// That makes it the one place where a golden table is genuinely ground truth
// rather than a snapshot of current behaviour, and where pinning it is
// stronger than any cross-check between implementations.
//
// # Why a cross-check would not be enough
//
// gofa's compiled table, NAIF's naif0012.tls and the discontinuities in IERS
// finals2000A are three *republications of one source*. Agreement between them
// is a consistency check, not validation — see [metrology.Reference]'s
// SharedAncestor field, which exists to keep exactly this distinction visible
// in the accuracy record. If all three misquoted IERS identically, every
// cross-check in the repository would still pass.
//
// # Provenance
//
// Extracted from NAIF's naif0012.tls and then verified entry by entry against
// the IERS Data Center timescale service, which is IERS's own answer rather
// than a republication: all 28 agreed, 0 disagreed. TestIERSOracleMatchesTheRecord
// (build tag validation, in this package) re-runs that verification against the
// live service.
//
// When a new leap second is announced, append it here — that is the deliberate
// human step, and it is the point at which someone reads the Bulletin C.
var leapSecondRecord = []leapStep{
	{1972, 1, 1, 10}, {1972, 7, 1, 11}, {1973, 1, 1, 12}, {1974, 1, 1, 13},
	{1975, 1, 1, 14}, {1976, 1, 1, 15}, {1977, 1, 1, 16}, {1978, 1, 1, 17},
	{1979, 1, 1, 18}, {1980, 1, 1, 19}, {1981, 7, 1, 20}, {1982, 7, 1, 21},
	{1983, 7, 1, 22}, {1985, 7, 1, 23}, {1988, 1, 1, 24}, {1990, 1, 1, 25},
	{1991, 1, 1, 26}, {1992, 7, 1, 27}, {1993, 7, 1, 28}, {1994, 7, 1, 29},
	{1996, 1, 1, 30}, {1997, 7, 1, 31}, {1999, 1, 1, 32}, {2006, 1, 1, 33},
	{2009, 1, 1, 34}, {2012, 7, 1, 35}, {2015, 7, 1, 36}, {2017, 1, 1, 37},
}

// TestLeapSecondRecordIsWellFormed checks the pinned table against the two
// properties the published record has by construction, before anything is
// compared against it.
//
// A golden table nobody validates is just a second implementation. These two
// assertions are what make a transcription slip fail here rather than silently
// become the reference every other test trusts.
func TestLeapSecondRecordIsWellFormed(t *testing.T) {
	t.Parallel()

	if len(leapSecondRecord) != 28 {
		t.Fatalf("the record has %d entries, expected 28; if a leap second was "+
			"announced, update this count deliberately rather than letting it drift",
			len(leapSecondRecord))
	}

	for i, s := range leapSecondRecord {
		// Every announced step is exactly +1 s. There has never been a
		// negative or multi-second leap second, and the ITU rules do not
		// permit one.
		if i > 0 {
			if prev := leapSecondRecord[i-1]; s.deltaAT-prev.deltaAT != 1 {
				t.Errorf("%04d-%02d-%02d: ΔAT steps by %g, but every announced leap "+
					"second is exactly +1 s",
					s.year, s.month, s.day, s.deltaAT-prev.deltaAT)
			}
		}

		// Leap seconds occur only at the end of June or December, so a step
		// is dated the 1st of January or July.
		if s.day != 1 || (s.month != 1 && s.month != 7) {
			t.Errorf("%04d-%02d-%02d is not a 1 January or 1 July step; leap seconds "+
				"are only inserted at the end of June or December",
				s.year, s.month, s.day)
		}
	}
}

// TestGofaMatchesTheLeapSecondRecord validates the table astrogo actually uses
// against the published record.
//
// gofa's Dat is the single source of ΔAT for the whole library — Time.TAI()
// calls it, and every UTC-based conversion goes through that. Nothing else
// checked it against anything.
func TestGofaMatchesTheLeapSecondRecord(t *testing.T) {
	t.Parallel()

	for _, s := range leapSecondRecord {
		got, status := gofaext.Dat(s.year, s.month, s.day, 0)
		if status < 0 {
			t.Errorf("%04d-%02d-%02d: gofa Dat failed with status %d",
				s.year, s.month, s.day, status)

			continue
		}

		if math.Abs(got-s.deltaAT) > 1e-9 {
			t.Errorf("%04d-%02d-%02d: gofa says ΔAT = %g, the published record says %g.\n"+
				"  These are the same administrative quantity, so this is not a "+
				"tolerance question — one of them is simply wrong.",
				s.year, s.month, s.day, got, s.deltaAT)
		}
	}
}

// TestLeapSecondBoundaryConvention pins what happens *at* a step, which is the
// part every other test in this repository deliberately avoids.
//
// TestLeapSecondSourcesAgree samples a day after each step precisely so that a
// half-open-interval disagreement cannot masquerade as a table mismatch. That
// keeps it a clean staleness tripwire and leaves the boundary — the one instant
// where implementations actually differ — unasserted.
//
// # The convention, confirmed against IERS
//
// The IERS timescale service was queried across two steps and answers:
//
//	2016-12-31 23:59:59 -> 36
//	2016-12-31 23:59:60 -> 36     the leap second itself carries the OLD value
//	2017-01-01 00:00:00 -> 37     the step instant carries the NEW value
//	2015-06-30 23:59:59 -> 35
//	2015-07-01 00:00:00 -> 36
//
// So the interval is half-open: [step, next step) takes the new ΔAT, and the
// inserted second belongs to the old one. gofa agrees, which is now recorded
// here rather than assumed.
func TestLeapSecondBoundaryConvention(t *testing.T) {
	t.Parallel()

	for i, s := range leapSecondRecord {
		if i == 0 {
			continue // nothing precedes the first entry
		}

		before := leapSecondRecord[i-1].deltaAT

		// The last moment of the previous interval: 23:59:59 on the day
		// before the step. Expressed as a fraction of the previous day.
		py, pm, pd := dayBefore(s.year, s.month, s.day)

		got, status := gofaext.Dat(py, pm, pd, 86399.0/86400.0)
		if status < 0 {
			t.Errorf("%04d-%02d-%02d 23:59:59: gofa Dat failed with status %d", py, pm, pd, status)

			continue
		}

		if math.Abs(got-before) > 1e-9 {
			t.Errorf("%04d-%02d-%02d 23:59:59 (the second before the step): gofa says %g, "+
				"expected %g — the old value must still apply right up to the step",
				py, pm, pd, got, before)
		}

		// The inserted second itself, 23:59:60, still carries the old value.
		got, status = gofaext.Dat(py, pm, pd, 1.0)
		if status < 0 {
			t.Errorf("%04d-%02d-%02d 23:59:60: gofa Dat failed with status %d", py, pm, pd, status)

			continue
		}

		if math.Abs(got-before) > 1e-9 {
			t.Errorf("%04d-%02d-%02d 23:59:60 (the leap second itself): gofa says %g, "+
				"expected %g — IERS reports the old value for the inserted second",
				py, pm, pd, got, before)
		}

		// The step instant takes the new value.
		got, status = gofaext.Dat(s.year, s.month, s.day, 0)
		if status < 0 {
			continue // already reported by TestGofaMatchesTheLeapSecondRecord
		}

		if math.Abs(got-s.deltaAT) > 1e-9 {
			t.Errorf("%04d-%02d-%02d 00:00:00 (the step instant): gofa says %g, expected %g "+
				"— the interval is half-open, so the step instant takes the new value",
				s.year, s.month, s.day, got, s.deltaAT)
		}
	}
}

// dayBefore returns the calendar day preceding a 1 January or 1 July step,
// which is all this file needs.
func dayBefore(year, month, day int) (int, int, int) {
	if day != 1 {
		return year, month, day - 1
	}

	if month == 1 {
		return year - 1, 12, 31
	}

	return year, month - 1, 30 // 1 July is preceded by 30 June
}
