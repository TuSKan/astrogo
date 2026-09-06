package satellite_test

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/satellite"
)

// tleChecksum recomputes a line's modulo-10 check digit, so a test can corrupt
// a field and hand ValidateTLE a set whose checksum is still correct.
//
// Reimplemented here rather than exported from the package: a checksum-valid
// corruption is a test tool, not something the library should offer.
func tleChecksum(line string) int {
	sum := 0

	for _, c := range line {
		switch {
		case c >= '0' && c <= '9':
			sum += int(c - '0')
		case c == '-':
			sum++
		}
	}

	return sum % 10
}

// rewriteField replaces line[start:end] with replacement and repairs the check
// digit, so the result passes every structural test and differs from the
// published set only in that field's contents.
//
// For the corruption tests that combination is the whole point: it is exactly
// what a checksum cannot see. It also builds the padded-field fixture below,
// which is a legal set rather than a corrupt one.
func rewriteField(t *testing.T, line string, start, end int, replacement string) string {
	t.Helper()

	if len(replacement) != end-start {
		t.Fatalf("replacement %q is %d chars, need %d — the corruption must not change the line length",
			replacement, len(replacement), end-start)
	}

	out := line[:start] + replacement + line[end:]
	out = out[:len(out)-1] + strconv.Itoa(tleChecksum(out[:len(out)-1]))

	if len(out) != len(line) {
		t.Fatalf("corrupted line is %d chars, want %d", len(out), len(line))
	}

	return out
}

// TestValidateTLERejectsANonNumericField covers every field the SGP4 backend
// parses, because the consequence of missing one is not a wrong answer.
//
// joshuaferrara/go-satellite reads its twelve numeric fields through helpers
// that call log.Fatal — os.Exit(1) — on a parse failure. There is no error and
// no panic to recover: one bad element set ends the caller's process. A
// modulo-10 checksum cannot prevent this, since letters and spaces contribute
// nothing to the sum, so a field replaced by text can still check out.
//
// Each case below is checksum-valid and structurally perfect. If ValidateTLE
// stops covering one, that case reaches strconv inside the backend.
func TestValidateTLERejectsANonNumericField(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		line       int // 1, 2, or 0 for both — see "satellite number"
		start, end int
		replace    string
	}{
		// Both lines, not one: corrupting line 1's satellite number alone is
		// caught by the earlier "these lines describe different satellites"
		// check, so the case would pass without the numeric check existing at
		// all. Corrupting both identically keeps that check satisfied and
		// leaves this as the only thing that can refuse the set. Confirmed by
		// deleting the numeric check and watching this case still pass.
		{"satellite number", 0, 2, 7, "ABCDE"},
		{"epoch year", 1, 18, 20, "XX"},
		{"epoch day", 1, 20, 32, "  bad.51782 "},
		{"first derivative", 1, 33, 43, " -.000x2182"[:10]},
		{"second derivative", 1, 44, 52, "  000Q0-0"[:8]},
		{"B* drag term", 1, 53, 61, "-116Z6-4"},
		{"inclination", 2, 8, 16, " 5x.6416"},
		{"raan", 2, 17, 25, "247.46Q7"},
		{"eccentricity", 2, 26, 33, "00067z3"},
		{"argument of perigee", 2, 34, 42, "130.53Y0"},
		{"mean anomaly", 2, 43, 51, "325.0Z88"},
		{"mean motion", 2, 52, 63, "1X.72125391"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			l1, l2 := valladoLine1, valladoLine2

			if c.line == 1 || c.line == 0 {
				l1 = rewriteField(t, l1, c.start, c.end, c.replace)
			}

			if c.line == 2 || c.line == 0 {
				l2 = rewriteField(t, l2, c.start, c.end, c.replace)
			}

			err := satellite.ValidateTLE(l1, l2)
			if !errors.Is(err, satellite.ErrMalformedTLE) {
				t.Fatalf("ValidateTLE accepted a non-numeric %s (err = %v).\n"+
					"  This set is checksum-valid, so nothing else will catch it, and the "+
					"backend calls os.Exit when strconv fails on this field.", c.name, err)
			}

			// The message must name the field. "malformed TLE" alone leaves the
			// caller to find which of twelve columns their feed mangled.
			if !strings.Contains(err.Error(), c.name) && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(c.name)) {
				t.Logf("note: error does not name %q verbatim: %v", c.name, err)
			}
		})
	}
}

// TestValidateTLEAcceptsSpacePaddedFields guards the one way this check could
// be too strict rather than too loose.
//
// The backend removes at most TWO spaces per field (strings.Replace with a
// count of 2), and this validation copies that count so that it accepts
// exactly what the backend accepts — no more, and no less. Getting the "no
// less" half wrong would reject real element sets in order to guard against
// corrupt ones.
//
// Two leading spaces is not a contrived case. Inclination, RAAN, argument of
// perigee and mean anomaly are all written %8.4f, so any value below 10 —
// every near-equatorial geostationary object, and any orbit whose node or
// perigee happens to sit in the first ten degrees — pads to exactly two. It
// cannot pad to three: three integer digits plus a point plus four decimals
// already fill the eight columns.
//
// The fixture is the published ISS set with those four fields rewritten to
// single-digit values and each checksum repaired — a legal element set, not a
// real object's. A real satellite is not needed to test a padding rule, and
// inventing one and calling it real would be worse than saying this.
func TestValidateTLEAcceptsSpacePaddedFields(t *testing.T) {
	t.Parallel()

	l2 := rewriteField(t, valladoLine2, 8, 16, "  5.1234") // inclination 5.1234°
	l2 = rewriteField(t, l2, 17, 25, "  8.7492")           // RAAN 8.7492°
	l2 = rewriteField(t, l2, 34, 42, "  1.5360")           // argument of perigee
	l2 = rewriteField(t, l2, 43, 51, "  5.0288")           // mean anomaly
	l2 = rewriteField(t, l2, 52, 63, " 1.00269781")        // mean motion, one rev/day

	for _, cols := range [][2]int{{8, 16}, {17, 25}, {34, 42}, {43, 51}} {
		if field := l2[cols[0]:cols[1]]; !strings.HasPrefix(field, "  ") {
			t.Fatalf("precondition: %q does not carry the two leading spaces this test exists for", field)
		}
	}

	if err := satellite.ValidateTLE(valladoLine1, l2); err != nil {
		t.Fatalf("a legal space-padded element set was rejected: %v\n"+
			"  Every one of these fields is written %%8.4f, so a value below 10 pads to "+
			"two spaces. Rejecting that refuses real data.", err)
	}

	sat, err := satellite.NewFromTLE("padded", valladoLine1, l2)
	if err != nil {
		t.Fatalf("NewFromTLE: %v", err)
	}

	// MeanMotion comes from the same parse the validation performs, so this
	// also confirms the field was read from the right columns and that the
	// leading space did not survive into strconv.
	if mm := sat.MeanMotion; mm < 1.0026 || mm > 1.0028 {
		t.Errorf("MeanMotion = %v rev/day, want 1.00269781 as written in columns 53-63", mm)
	}
}

// TestNewFromTLESurvivesANonNumericField is the assertion the test above
// cannot make: that the process is still running afterward.
//
// A regression here does not fail an assertion — it calls os.Exit(1) from
// inside the backend and takes the test binary with it, which would look like
// an unrelated crash. Running the call in a subprocess turns "did not exit"
// into something this test can actually observe and report.
func TestNewFromTLESurvivesANonNumericField(t *testing.T) {
	t.Parallel()

	if os.Getenv("ASTROGO_TLE_EXIT_PROBE") == "1" {
		l2 := rewriteField(t, valladoLine2, 52, 63, "1X.72125391")

		if _, err := satellite.NewFromTLE("probe", valladoLine1, l2); err != nil {
			t.Logf("REFUSED: %v", err)

			return
		}

		t.Log("ACCEPTED")

		return
	}

	// The command is this test binary re-invoked on one of its own tests; no
	// part of it comes from outside the process.
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run", "^"+t.Name()+"$", "-test.v")

	cmd.Env = append(os.Environ(), "ASTROGO_TLE_EXIT_PROBE=1")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the child process died on a non-numeric mean motion: %v\n"+
			"  This is the failure mode the guard exists for — os.Exit(1) from inside "+
			"the SGP4 backend, with no error and nothing to recover.\n%s", err, out)
	}

	if !strings.Contains(string(out), "REFUSED:") {
		t.Errorf("the child did not refuse the corrupted set:\n%s", out)
	}
}
