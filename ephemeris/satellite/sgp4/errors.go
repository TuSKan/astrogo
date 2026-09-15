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

// Propagation errors, raised while evaluating the model.
//
// # Two of these come with a usable state
//
// [ErrDecayed] and [ErrKeplerNotConverged] are returned alongside the position
// and velocity that were computed, because both describe a result that exists
// and should not be trusted — which is a different thing from a computation
// that could not be performed. Every other error here leaves both vectors zero.
//
// `if err != nil { return }` remains the right handling for all of them. A
// caller who wants to look at a decayed satellite's position has to ask for it
// deliberately, which is the correct amount of friction.
var (
	// ErrMeanMotion indicates the mean motion went non-positive under drag.
	// Vallado's error 2.
	ErrMeanMotion = errors.New("sgp4: mean motion is not positive")

	// ErrEccentricity indicates the mean eccentricity left [0, 1) under drag —
	// the orbit is no longer elliptical and the theory no longer applies.
	// Vallado's error 1.
	ErrEccentricity = errors.New("sgp4: mean eccentricity is outside [0, 1)")

	// ErrSemiLatusRectum indicates a negative semi-latus rectum, which is not a
	// conic. Vallado's error 4.
	ErrSemiLatusRectum = errors.New("sgp4: semi-latus rectum is negative")

	// ErrDecayed indicates the model puts the satellite below the Earth's
	// surface. Vallado's error 6, and the one his code treats as advisory
	// rather than fatal: the state is computed and returned.
	//
	// It is a statement about the model, not about the object. SGP4 has no
	// atmosphere below its drag parameterisation and will happily continue
	// propagating an orbit that reentered years ago.
	ErrDecayed = errors.New("sgp4: the model places the satellite below the Earth's surface")

	// ErrKeplerNotConverged indicates the model's Kepler iteration used its ten
	// passes without reaching its tolerance.
	//
	// This has no counterpart in the reference, which gives up in silence and
	// uses whatever it has. That silence is not harmless: it is the whole of
	// astrogo's measured divergence on satellite 23333, where Vallado's own
	// note says the Spacetrack Report #3 solver stops converging past about 200
	// minutes. Reporting it changes no number and turns an unexplained
	// disagreement into a message.
	ErrKeplerNotConverged = errors.New("sgp4: the Kepler iteration did not converge")

	// ErrDeepSpace indicates an element set whose period reaches SGP4's
	// 225-minute deep-space threshold, where the model hands over to SDP4.
	//
	// Temporary. The deep-space path is the next step of the build described in
	// docs/sgp4.md, and this sentinel goes away with it — it exists so that a
	// caller who hits the gap gets something they can match on rather than a
	// bare string.
	ErrDeepSpace = errors.New("sgp4: deep-space (SDP4) propagation is not implemented yet")
)
