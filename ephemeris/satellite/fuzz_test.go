package satellite_test

import (
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/satellite"
)

// This file fuzzes the TLE entry points against corrupted and adversarial
// element sets. Every seed is a string literal — no checked-in fixture — so the
// seed corpus runs as an ordinary test under `go test ./...` and is in every CI
// run for free. Extended fuzzing is a manual, periodic step; see CLAUDE.md for
// the invocation and the -fuzzminimizetime flag it carries (#140).
//
// # The property here is unusually literal
//
// Elsewhere in the module, "must never panic or hang" is a way of saying that a
// parser should return an error rather than misbehave. Here it is the actual
// observed failure: the SGP4 backend parses its twelve numeric TLE fields
// through helpers that call log.Fatal, which is os.Exit(1) from inside a
// library — no error, no panic, and nothing to recover. A checksum-valid
// element set with a non-numeric field ended the caller's process until
// ValidateTLE was taught to parse every field the backend will (#177/#180).
//
// So this target is not looking for a hypothetical crash. It is continuously
// re-checking a guard against a defect that has already been demonstrated, on
// the exact input class that triggers it. If the guard ever develops a hole,
// the worker does not report a crasher — it exits, and the fuzz run fails with
// a strconv error in the log rather than a testdata/fuzz entry. That is worth
// knowing before reading the output.

// The canonical ISS element set from Vallado's SGP4 test suite, which is the
// reference every implementation checks itself against.
const (
	fuzzLine1 = "1 25544U 98067A   08264.51782528 -.00002182  00000-0 -11606-4 0  2927"
	fuzzLine2 = "2 25544  51.6416 247.4627 0006703 130.5360 325.0288 15.72125391563537"
)

// FuzzValidateTLE drives the checker itself, which is astrogo's only defence
// between an untrusted element set and the backend.
func FuzzValidateTLE(f *testing.F) {
	f.Add(fuzzLine1, fuzzLine2)

	// A real geostationary-shaped pair: values below 10 leave two leading
	// spaces in the %8.4f columns, which the backend's own two-space Replace
	// only just handles.
	f.Add(
		"1 19548U 88091B   24001.44443119 -.00000283  00000-0  00000-0 0  9993",
		"2 19548   3.7361   8.7492 0038421 351.2262   8.6521  1.00269781128644",
	)

	// The two lines swapped, and two lines from different satellites: the
	// substitutions no checksum can see.
	f.Add(fuzzLine2, fuzzLine1)
	f.Add(fuzzLine1, "2 20580 028.4699 288.8102 0002739 279.8160 080.2317 15.09299865538277")

	// Too short, too long, empty, and all-numeric — the four shapes a
	// fixed-column parser indexes past the end of.
	f.Add("1 25544", "2 25544")
	f.Add(fuzzLine1+"extra", fuzzLine2+"extra")
	f.Add("", "")
	f.Add(strings.Repeat("0", 69), strings.Repeat("0", 69))

	// Non-numeric in a numeric column, with the checksum left alone: the class
	// that reached strconv inside the backend.
	f.Add(fuzzLine1, "2 25544  51.6416 247.4627 0006703 130.5360 325.0288 1X.72125391563537"[:69])

	f.Fuzz(func(t *testing.T, line1, line2 string) {
		err := satellite.ValidateTLE(line1, line2)
		if err != nil {
			return
		}

		// ValidateTLE said yes, so NewFromTLE must not be the thing that
		// discovers otherwise. This is the assertion: anything the checker
		// accepts, the constructor has to survive.
		//
		// It can still legitimately fail — SGP4 rejects an orbit it cannot
		// initialise, which is an error and not a defect. What it must not do
		// is terminate the process, and reaching this line at all is the check.
		sat, err := satellite.NewFromTLE("fuzz", line1, line2)
		if err != nil {
			return
		}

		if sat == nil {
			t.Fatal("NewFromTLE returned a nil satellite and a nil error")
		}

		// A constructed satellite must be usable. OrbitalPeriod divides by
		// mean motion, which a fuzzed element set can drive to zero or to a
		// denormal.
		_ = sat.OrbitalPeriod()
	})
}

// FuzzNewFromTLE reaches the constructor without the checker in front of it,
// so the fuzzer can explore inputs ValidateTLE rejects today.
//
// Kept separate rather than folded into the target above: this one is allowed
// to fail in ways that one is not, and mixing them would hide which guard a
// crasher belongs to. The only property asserted here is that the call returns.
func FuzzNewFromTLE(f *testing.F) {
	f.Add(fuzzLine1, fuzzLine2)
	f.Add("1 00005U 58002B   00179.78495062  .00000023  00000-0  28098-4 0  4753",
		"2 00005  34.2682 348.7242 1859667 331.7664  19.3264 10.82419157413667")
	f.Add("", "")

	// Deliberately checksum-invalid, so this explores the space the target
	// above turns away at the door.
	f.Add("1 33333U 05037B   05333.02012661  .25992681  00000-0  24476-3 0  1534",
		"2 33333  96.4736 157.9986 9950000 244.0492 110.6523  4.00004038 10708")

	f.Fuzz(func(_ *testing.T, line1, line2 string) {
		sat, err := satellite.NewFromTLE("fuzz", line1, line2)
		if err != nil || sat == nil {
			return
		}

		_ = sat.OrbitalPeriod()
		_ = sat.Name
	})
}
