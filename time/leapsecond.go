package time

import (
	"errors"
	"fmt"
	"math"
	"sync/atomic"

	"github.com/TuSKan/astrogo/internal/gofaext"
)

// LeapSecond is one step in the published ΔAT (TAI−UTC) record: the instant a
// value takes effect, and the value.
//
// Dates are the 1 January or 1 July on which the new count applies, which is
// the form IERS announces and every published table uses.
type LeapSecond struct {
	// Year, Month and Day are the UTC calendar date the value takes effect.
	Year, Month, Day int

	// DeltaAT is TAI−UTC in seconds from that instant until the next entry.
	DeltaAT float64
}

// Errors returned by [RegisterLeapSeconds].
var (
	// ErrLeapSecondConflict is returned when a table contradicts the built-in
	// one rather than extending it.
	ErrLeapSecondConflict = errors.New("time: leap-second table contradicts the built-in record")

	// ErrLeapSecondOrder is returned for a table that is not in ascending date
	// order, or whose steps are not ±1 s.
	ErrLeapSecondOrder = errors.New("time: leap-second table is not a well-formed record")
)

// leapTable is an installed table together with where it came from. Replaced
// wholesale, never mutated in place, so a reader always sees a consistent pair.
type leapTable struct {
	entries []LeapSecond
	source  string
}

// leapRegistry holds the process-wide override, or nil for gofa's table.
//
// Atomic rather than mutex-guarded because [deltaAT] runs on the hot path —
// every UTC↔TAI↔TT conversion, and coord.Context does several per epoch —
// while a write happens at most once or twice in a process's life.
//
// Measured on BenchmarkUTCToTAI, against 41.2 ns/op before this file existed:
// an RWMutex read-lock pair cost 8.7 ns (50.1 ns/op, +22%) for a lock that is
// never contended and guards a value that almost never changes. The atomic
// load costs 2.0 ns (43.2 ns/op), and BenchmarkFullRoundTrip returns to its
// baseline 285 ns/op. Do not reintroduce a mutex here without re-running those.
var leapRegistry atomic.Pointer[leapTable]

// RegisterLeapSeconds installs a ΔAT table for the whole process, to be used
// in preference to the one compiled into gofa.
//
// # Why this exists
//
// gofa's Dat carries a table hardcoded in its source and pinned by go.mod. A
// newly announced leap second reaches astrogo only through a dependency bump
// and a release. A published table — IERS Bulletin C, or the DELTA_AT block of
// a NAIF leap-second kernel — is a data file that can be refreshed. This lets
// the fresher source win without astrogo carrying two of them in use at once:
// one table is active process-wide at any moment.
//
// source names where the table came from, for [LeapSecondSource].
//
// # Superset-only, and why that is not negotiable
//
// A table may **add** entries after the built-in record's last step. It may not
// contradict one the built-in record already has: that returns
// [ErrLeapSecondConflict] and changes nothing.
//
// Without that rule this has an ordering hazard worse than EOP's. A missing
// EOP is a documented degradation — zero EOP, a warning, sub-arcsecond. A
// missing leap second is *correct until it isn't*, and a process that computed
// epochs before a table registered and after would silently get answers a full
// second apart. Restricted to extensions, a late registration can only ever
// add future entries, so every epoch before the built-in table's end is
// unaffected regardless of when registration happens — which is essentially
// all real use.
//
// The record is also nearly closed: CGPM Resolution 4 (2022) ends leap seconds
// by 2035, so this can gain at most one or two more entries, possibly
// including the first negative one, and then stop for good.
func RegisterLeapSeconds(table []LeapSecond, source string) error {
	if err := validateLeapTable(table); err != nil {
		return err
	}

	if err := checkAgainstBuiltin(table); err != nil {
		return err
	}

	leapRegistry.Store(&leapTable{
		entries: append([]LeapSecond(nil), table...),
		source:  source,
	})

	return nil
}

// ResetLeapSeconds discards any registered table, restoring gofa's built-in
// one. Intended for tests, mirroring [ResetEOP].
func ResetLeapSeconds() {
	leapRegistry.Store(nil)
}

// LeapSecondSource reports where the ΔAT currently in use comes from:
// "builtin" for gofa's compiled table, or whatever name was passed to
// [RegisterLeapSeconds].
//
// Lets a caller — a test in particular — assert which table ran, rather than
// inferring it from a numeric result.
func LeapSecondSource() string {
	if t := leapRegistry.Load(); t != nil {
		return t.source
	}

	return "builtin"
}

// deltaAT returns TAI−UTC in seconds at a UTC calendar instant.
//
// The registered table wins where it has an entry at or before the instant;
// otherwise gofa's own table answers. Both agree below the built-in record's
// last step, because registration refuses a table that disagrees there, so
// this can only differ for epochs gofa does not know about.
func deltaAT(y, m, d int, fd float64) float64 {
	var table []LeapSecond
	if t := leapRegistry.Load(); t != nil {
		table = t.entries
	}

	if len(table) == 0 {
		dat, _ := gofaext.Dat(y, m, d, fd)

		return dat
	}

	key := leapKey(y, m, d)

	// Entries are ascending, so the last one at or before the instant is the
	// one in force. Before the first entry the table says nothing and gofa
	// answers — that is the pre-1972 era, where ΔAT is not defined at all.
	found := false

	var value float64

	for _, e := range table {
		if leapKey(e.Year, e.Month, e.Day) > key {
			break
		}

		value, found = e.DeltaAT, true
	}

	if !found {
		dat, _ := gofaext.Dat(y, m, d, fd)

		return dat
	}

	return value
}

// leapKey orders a calendar date as a single comparable integer.
func leapKey(y, m, d int) int { return (y*100+m)*100 + d }

// validateLeapTable checks a table against the two properties the published
// record has by construction.
func validateLeapTable(table []LeapSecond) error {
	if len(table) == 0 {
		return fmt.Errorf("%w: empty", ErrLeapSecondOrder)
	}

	for i, e := range table {
		if i == 0 {
			continue
		}

		prev := table[i-1]
		if leapKey(e.Year, e.Month, e.Day) <= leapKey(prev.Year, prev.Month, prev.Day) {
			return fmt.Errorf("%w: %04d-%02d-%02d does not follow %04d-%02d-%02d",
				ErrLeapSecondOrder, e.Year, e.Month, e.Day, prev.Year, prev.Month, prev.Day)
		}

		// A leap second is one second, in either direction. Positive is all
		// that has ever occurred, but the ITU-R has permitted a negative one
		// since 1972 and it is no longer considered unlikely — so a table
		// carrying the first one must not be rejected for carrying it.
		if step := e.DeltaAT - prev.DeltaAT; math.Abs(step) != 1 {
			return fmt.Errorf("%w: %04d-%02d-%02d steps by %g, but a leap second is "+
				"exactly one second, inserted or skipped",
				ErrLeapSecondOrder, e.Year, e.Month, e.Day, step)
		}
	}

	return nil
}

// checkAgainstBuiltin rejects a table that disagrees with gofa's within the
// era gofa knows about.
//
// # Telling an extension from a contradiction
//
// Not by gofa's status code. Dat reports "dubious year" only well past its
// table's end, so an entry that is genuinely new — a leap second announced for
// next January — still falls inside the confident range and would be
// mistaken for a disagreement.
//
// The discriminator is gofa's own last step, found by asking it. Up to and
// including that date the two must match exactly; after it, a difference is
// the new information this whole mechanism exists to carry.
func checkAgainstBuiltin(table []LeapSecond) error {
	last := builtinLastStep()

	for _, e := range table {
		if leapKey(e.Year, e.Month, e.Day) > last {
			continue // beyond what gofa knows: an extension
		}

		got, status := gofaext.Dat(e.Year, e.Month, e.Day, 0)
		if status < 0 {
			continue // outside gofa's supported range entirely
		}

		if math.Abs(got-e.DeltaAT) > 1e-9 {
			return fmt.Errorf("%w: at %04d-%02d-%02d the table says %g and the built-in "+
				"record says %g. A registration may extend the record, never restate it "+
				"differently — one of the two is stale",
				ErrLeapSecondConflict, e.Year, e.Month, e.Day, e.DeltaAT, got)
		}
	}

	return nil
}

// builtinLastStep is the date of the final step in gofa's compiled table, as a
// leapKey.
//
// Found by asking rather than hardcoded, so a gofa upgrade that adds an entry
// moves this automatically instead of turning the new entry into a conflict.
func builtinLastStep() int {
	const (
		firstYear = 1972
		lastYear  = 2100
	)

	var (
		lastKey  int
		previous float64
	)

	previous, _ = gofaext.Dat(firstYear, 1, 1, 0)

	for y := firstYear; y <= lastYear; y++ {
		for _, m := range [2]int{1, 7} {
			got, status := gofaext.Dat(y, m, 1, 0)
			if status < 0 {
				continue
			}

			if math.Abs(got-previous) > 1e-9 {
				lastKey = leapKey(y, m, 1)
			}

			previous = got
		}
	}

	return lastKey
}
