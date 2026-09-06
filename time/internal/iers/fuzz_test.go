package iers

import (
	"strings"
	"testing"
)

// This file fuzzes the IERS finals2000A parser. Every seed is a string literal
// — no checked-in fixture — so the seed corpus runs under `go test ./...` and is
// in every CI run for free. Extended fuzzing is manual and periodic; see
// CLAUDE.md, including the -fuzzminimizetime flag (#140).
//
// # Why this one matters out of proportion to its size
//
// finals2000A is fixed-width: every field is a column range, and the parser
// slices by index. That is the shape that indexes past the end of a truncated
// line, and the bulletin arrives over the network from an endpoint astrogo does
// not control.
//
// The consequence is what makes it worth fuzzing rather than the crash. A
// malformed bulletin does not stop the library — it degrades every epoch
// calculation in it, silently, because Time.EOP has no error return and falls
// back to zero EOP with a single warning. A parser that accepted a corrupt file
// as a short one would be indistinguishable from a quiet night.
//
// In-package rather than iers_test, because time/internal/iers is unexported:
// nothing outside time/ can import it, so a fuzz target has to live here.

// A real finals2000A.all record, and the header-less format the file actually
// has: fixed columns, no delimiters, MJD in 8-15.
const realRecord = "73 1 2 41684.00 I  0.120733 0.009786  0.136966 0.015902  I 0.8084178 0.0002710" +
	"  0.0000 0.1916  P    -0.766    0.199    -0.720    0.300"

func FuzzParseFinals2000A(f *testing.F) {
	f.Add(realRecord)
	f.Add(realRecord + "\n" + realRecord)
	f.Add("")
	f.Add("\n\n\n")

	// Truncated at each of the boundaries the parser slices on, which is the
	// failure this target exists for.
	for _, n := range []int{1, 10, 16, 20, 40, 60, 79} {
		if n < len(realRecord) {
			f.Add(realRecord[:n])
		}
	}

	// Right length, wrong contents: spaces where numbers go, and letters.
	f.Add(strings.Repeat(" ", len(realRecord)))
	f.Add(strings.Repeat("x", len(realRecord)))

	// A prediction flag where a measurement flag belongs, and vice versa —
	// the two the parser branches on.
	f.Add(strings.Replace(realRecord, " I  0.120733", " P  0.120733", 1))

	// Numbers that parse but are not physical: EOP is arcseconds and seconds,
	// so these are far outside anything Earth does.
	f.Add(strings.Replace(realRecord, "0.120733", "9.999999", 1))

	// Very many records, to reach whatever the parser does with a large table.
	f.Add(strings.Repeat(realRecord+"\n", 200))

	f.Fuzz(func(t *testing.T, data string) {
		table, err := ParseFinals2000A(strings.NewReader(data))
		if err != nil {
			return
		}

		if table == nil {
			t.Fatal("ParseFinals2000A returned a nil table and a nil error")
		}

		// A table that parsed must be queryable. Lookup interpolates between
		// records and is where an out-of-order or single-record table would go
		// wrong, so it is driven at, inside and outside whatever coverage the
		// fuzzed input produced.
		start, end := table.Coverage()

		for _, mjd := range []float64{start - 1, start, (start + end) / 2, end, end + 1} {
			_, _ = table.EOP(mjd)
		}
	})
}
