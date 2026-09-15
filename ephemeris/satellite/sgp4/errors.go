package sgp4

import "errors"

// Element-set errors, raised while reading or validating an [Elements] value.
var (
	// ErrMalformedTLE indicates text that is not a well-formed two-line
	// element set: the wrong length, the wrong line numbers, two lines
	// describing different satellites, or a field that is not the number the
	// format says it is.
	ErrMalformedTLE = errors.New("sgp4: malformed TLE")

	// ErrChecksum indicates a TLE line whose last character does not match the
	// modulo-10 sum of the rest.
	//
	// Separate from ErrMalformedTLE, and checked by a separate function, for a
	// reason worth stating: a check digit is about whether the *text* arrived
	// intact, and every other field is about whether the *elements* make sense.
	// A feed that mangles check digits while transmitting correct elements is a
	// real thing, and so is a corrupted element that still checksums. Vallado's
	// own verification suite contains three element sets he hand-built to
	// exercise SGP4's error returns and never maintained the check digits of —
	// they are perfectly good inputs for their purpose and a checksum-enforcing
	// parser cannot read them.
	ErrChecksum = errors.New("sgp4: TLE checksum mismatch")

	// ErrElements indicates an element set that is structurally fine and
	// physically impossible — a negative mean motion, an eccentricity outside
	// [0, 1), an inclination outside [0, pi].
	ErrElements = errors.New("sgp4: element set is out of range")
)
