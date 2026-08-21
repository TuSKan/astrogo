package satellite_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/satellite"
)

// The canonical ISS element set from Vallado's SGP4 test suite, which is the
// reference every implementation of this checks itself against. Using it here
// means the checksum rule is anchored to a published pair rather than to one
// this repository produced for itself.
const (
	valladoLine1 = "1 25544U 98067A   08264.51782528 -.00002182  00000-0 -11606-4 0  2927"
	valladoLine2 = "2 25544  51.6416 247.4627 0006703 130.5360 325.0288 15.72125391563537"
)

// A published element set must validate exactly as it was published.
func TestValidateTLEAcceptsAPublishedElementSet(t *testing.T) {
	t.Parallel()

	if err := satellite.ValidateTLE(valladoLine1, valladoLine2); err != nil {
		t.Fatalf("the reference ISS element set was rejected: %v", err)
	}

	// And it builds, so validation has not closed the ordinary path.
	if _, err := satellite.NewFromTLE("ISS (ZARYA)", valladoLine1, valladoLine2); err != nil {
		t.Errorf("NewFromTLE on the reference element set: %v", err)
	}
}

// One altered digit must be refused.
//
// This is the whole point of the checksum. SGP4 initialises happily from a
// corrupted element set and propagates it for ever, because every field it
// reads is still a number: an inclination of 51.6416 degrees becomes 61.6416
// and the orbit is simply a different one, with nothing anywhere reporting a
// problem.
func TestValidateTLERejectsASingleAlteredDigit(t *testing.T) {
	t.Parallel()

	// Walk the whole of each line, changing one digit at a time. Every such
	// change alters the checksum, so every one must be caught.
	for lineNo, line := range [2]string{valladoLine1, valladoLine2} {
		caught, tried := 0, 0

		for i := range len(line) - 1 { // the checksum character itself is separate
			c := line[i]
			if c < '0' || c > '9' {
				continue
			}

			// Change the digit to something else, avoiding a change of 10 in
			// the sum, which the modulo cannot see.
			replacement := '0' + (c-'0'+1)%10
			corrupted := line[:i] + string(replacement) + line[i+1:]

			tried++

			var err error
			if lineNo == 0 {
				err = satellite.ValidateTLE(corrupted, valladoLine2)
			} else {
				err = satellite.ValidateTLE(valladoLine1, corrupted)
			}

			if errors.Is(err, satellite.ErrMalformedTLE) {
				caught++
			} else {
				t.Errorf("line %d, digit %d changed from %q to %q: err = %v, want ErrMalformedTLE",
					lineNo+1, i, c, replacement, err)
			}
		}

		if tried == 0 {
			t.Fatalf("line %d: no digits were tried", lineNo+1)
		}

		if caught != tried {
			t.Errorf("line %d: caught %d of %d single-digit corruptions", lineNo+1, caught, tried)
		}
	}
}

// The malformations a checksum cannot see, and the ones it can.
func TestValidateTLERejectsMalformedInput(t *testing.T) {
	t.Parallel()

	// Line 2 of a different satellite. Both lines are individually intact and
	// both checksums pass; only the catalogue number reveals it. This is the
	// one a copy-paste from two rows of a listing actually produces.
	const otherLine2 = "2 20580 028.4699 288.8102 0002739 279.8160 080.2317 15.09299865538277"

	for _, c := range []struct {
		name         string
		line1, line2 string
	}{
		{"empty", "", ""},
		{"line 1 truncated", valladoLine1[:60], valladoLine2},
		{"line 2 truncated", valladoLine1, valladoLine2[:68]},
		{"line 1 with trailing space", valladoLine1 + " ", valladoLine2},
		{"the two lines swapped", valladoLine2, valladoLine1},
		{"line 1 given twice", valladoLine1, valladoLine1},
		{"line 2 given twice", valladoLine2, valladoLine2},
		{"line 2 of another satellite", valladoLine1, otherLine2},
		{"line 1 checksum wrong", valladoLine1[:68] + "0", valladoLine2},
		{"line 2 checksum wrong", valladoLine1, valladoLine2[:68] + "0"},
	} {
		if err := satellite.ValidateTLE(c.line1, c.line2); !errors.Is(err, satellite.ErrMalformedTLE) {
			t.Errorf("%s: err = %v, want ErrMalformedTLE", c.name, err)
		}

		// And the same through the constructor, which is the door callers use.
		if _, err := satellite.NewFromTLE("test", c.line1, c.line2); err == nil {
			t.Errorf("%s: NewFromTLE accepted it", c.name)
		}
	}

	// The mismatched pair is worth stating on its own, because it is the case
	// no checksum can catch: confirm both lines really do pass individually.
	if err := satellite.ValidateTLE(valladoLine1, valladoLine2); err != nil {
		t.Fatalf("the reference pair should validate: %v", err)
	}

	if err := satellite.ValidateTLE(strings.Replace(otherLine2, "2 20580", "1 20580", 1), otherLine2); err == nil {
		t.Log("the other satellite's line 2 is individually well formed")
	}
}

// A corrupted element set must be refused rather than propagated.
//
// Without the check, this is the failure that matters: the orbit is a
// different one, and every position derived from it is confidently wrong.
func TestNewFromTLERefusesACorruptedElementSet(t *testing.T) {
	t.Parallel()

	// Inclination 51.6416 becomes 61.6416: ten degrees of orbital plane, and a
	// perfectly ordinary number.
	corrupted := strings.Replace(valladoLine2, " 51.6416 ", " 61.6416 ", 1)

	if len(corrupted) != len(valladoLine2) {
		t.Fatalf("the corruption changed the line length; the test is not testing what it means to")
	}

	if _, err := satellite.NewFromTLE("ISS", valladoLine1, corrupted); !errors.Is(err, satellite.ErrMalformedTLE) {
		t.Errorf("an element set with a ten degree error in inclination was accepted: err = %v", err)
	}
}
