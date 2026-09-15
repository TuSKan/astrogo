package sgp4

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/time"
)

// Elements is the mean element set SGP4 propagates.
//
// It is not an orbit. These values are the output of fitting observations
// through SGP4 itself, so they describe an orbit only in the sense that feeding
// them back to the same model reproduces where the object was seen. Reading
// [Elements.Eccentricity] as the eccentricity of an ellipse, or deriving a
// semi-major axis from [Elements.MeanMotion] by Kepler's third law, gives a
// number that looks right and is not: SGP4 removes the secular J2 term from the
// mean motion before it does anything else, which moves the perigee of a low
// orbit by about 1.4 km.
//
// # Units
//
// Angles are [angle.Angle], so the caller never has to know which fields a TLE
// stores in degrees. Everything else keeps the unit the element set is fitted
// in, because those are the units SGP4's coefficients are defined against and
// converting them here would only mean converting them back:
//
//   - [Elements.MeanMotion] is revolutions per day, and is the *Kozai* mean
//     motion.
//   - [Elements.MeanMotionDot] and [Elements.MeanMotionDDot] carry the TLE's
//     own halving and sixthing — they are n'/2 and n”/6, not n' and n”. That
//     is a trap worth a comment rather than a silent convention: undoing it
//     here would leave every caller who knows the TLE format wondering which
//     one they have.
//   - [Elements.BStar] is in inverse Earth radii.
//
// Neither derivative is used by SGP4 at all — the model takes its drag entirely
// from BStar. They are carried because they are part of the element set and a
// caller may want to report them.
type Elements struct {
	// Name is the object's common name, from a TLE's optional title line.
	// Empty when the source carried none; SGP4 never reads it.
	Name string

	// NORAD is the catalogue number.
	NORAD int

	// Designator is the international launch designator in the TLE's own
	// spelling — "98067A", a two-digit launch year, a three-digit launch
	// number of that year, and a piece. Not expanded to the canonical
	// "1998-067A": the century is inferable but this field is what the source
	// said.
	Designator string

	// Classification is 'U' (unclassified), 'C' (classified) or 'S' (secret).
	Classification byte

	// ElementSet is the element set number, incremented each time the object
	// is refitted, and RevAtEpoch the revolution number at epoch.
	ElementSet int
	RevAtEpoch int

	// Epoch is the instant the elements describe, in UTC.
	//
	// A TLE writes it as a two-digit year and a fractional day of year, so it
	// resolves to about 0.9 ms and nothing finer. The two-digit year pivots at
	// 57: 57-99 are 1957-1999, 00-56 are 2000-2056. That is the format's own
	// rule and it expires in 2057.
	Epoch time.Time

	// Inclination is measured from the equator, in [0, pi].
	Inclination angle.Angle
	// RAAN is the right ascension of the ascending node.
	RAAN angle.Angle
	// ArgPerigee is the argument of perigee.
	ArgPerigee angle.Angle
	// MeanAnomaly is the mean anomaly at Epoch.
	MeanAnomaly angle.Angle

	// Eccentricity is dimensionless, in [0, 1).
	Eccentricity float64

	// MeanMotion is the Kozai mean motion in revolutions per day.
	MeanMotion float64

	// MeanMotionDot is n'/2 in revolutions per day squared, and MeanMotionDDot
	// is n''/6 in revolutions per day cubed — the TLE's own conventions. SGP4
	// reads neither.
	MeanMotionDot  float64
	MeanMotionDDot float64

	// BStar is the drag-like coefficient, in inverse Earth radii. This is the
	// only drag term SGP4 uses, and it can legitimately be negative.
	BStar float64
}

// The bounds Validate enforces. Named so the error messages and the tests can
// quote the same numbers.
const (
	// maxEccentricity is the open upper bound: SGP4's own propagation refuses
	// e >= 1 at runtime, and an element set that starts there was never
	// elliptical.
	maxEccentricity = 1.0

	// minEccentricity is 0 and the bound is inclusive. SGP4 clamps a perfectly
	// circular orbit to 1e-6 internally to avoid a division, which is its
	// business; an element set is entitled to say zero.
	minEccentricity = 0.0
)

// Validate reports whether the element set is physically possible.
//
// # Possible, not plausible
//
// The distinction matters more than it looks. Vallado's verification suite
// contains an element set with a mean motion of 0.00001 revolutions per day — a
// period of 274 years — built deliberately to drive SGP4 into one of its error
// returns. It is absurd and it is a legitimate input, and a validator that
// rejected it would make the model's own error paths untestable.
//
// So this refuses only what cannot be an element set at all: a mean motion that
// is zero or negative (there is no such orbit and SGP4 divides by it), an
// eccentricity outside [0, 1) (SGP4 is an elliptical theory), an inclination
// outside [0, pi] (the range the angle is defined on), and a zero epoch (an
// element set has to be *for* some time). Everything else is the propagator's
// to complain about, at the point where it actually fails.
//
// [ParseTLE] calls this, so a parsed element set is already validated.
func (e Elements) Validate() error {
	switch {
	case e.MeanMotion <= 0:
		return fmt.Errorf("%w: mean motion is %g revolutions per day, which is not an orbit",
			ErrElements, e.MeanMotion)

	case math.IsNaN(e.Eccentricity) || e.Eccentricity < minEccentricity || e.Eccentricity >= maxEccentricity:
		return fmt.Errorf("%w: eccentricity is %g, outside [%g, %g) — SGP4 is an elliptical theory",
			ErrElements, e.Eccentricity, minEccentricity, maxEccentricity)

	case math.IsNaN(e.Inclination.Radians()) ||
		e.Inclination.Radians() < 0 || e.Inclination.Radians() > math.Pi:
		return fmt.Errorf("%w: inclination is %g degrees, outside [0, 180]",
			ErrElements, e.Inclination.Degrees())

	case math.IsNaN(e.BStar) || math.IsInf(e.BStar, 0):
		return fmt.Errorf("%w: B* is %g", ErrElements, e.BStar)

	case e.Epoch.IsZero():
		return fmt.Errorf("%w: epoch is unset — an element set describes a particular instant",
			ErrElements)
	}

	return nil
}
