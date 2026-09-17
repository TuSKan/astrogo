package unit

import (
	"fmt"
	"math"
)

// Length is a distance, stored in metres.
//
// # Why a named float64 and not a [Quantity]
//
// Because the mistake this prevents is not a dimensional one. Nobody passes a
// duration where a distance goes; what astrogo's API got wrong was units
// *within* a dimension — [coord.ICRS]'s distance held astronomical units on an
// ephemeris path and kilometres on a satellite one, with nothing in the
// signature saying which, and a caller could not tell them apart.
//
// A Quantity would catch that and more, and costs more than it needs to: it is
// 56 bytes against 8, and measured over a 1000-element batch it is 14.3x slower
// (27.0 µs against 1.89 µs) because seven times the working set is seven times
// the cache pressure. A named float64 is byte-for-byte and instruction-for-
// instruction identical to the float64 it replaces — measured at 1.88 ns
// against 1.88 ns scalar, and 1887 ns against 1888 ns over that same batch.
//
// Neither allocates. The Quantity cost is width, not the heap, and both satisfy
// the allocs-per-op contracts in coord, time and atmosphere.
//
// This is the pattern [angle.Angle] already uses, in the same hot paths, under
// the same contracts. Length and [Velocity] finish what that started.
//
// # Why metres
//
// Not a preference: [Meter] is declared here with ScaleFactor 1.0 and every
// other unit's ScaleFactor is expressed against it, so this package has treated
// metres as canonical since it was written. Storing anything else would mean
// Length and Quantity disagreed about what 1.0 means inside one package.
//
// Metres are not a precision compromise either. float64 carries about 16
// significant digits *relative* to the magnitude, so a galactic distance held
// in metres and the same distance held in parsecs are equally precise — 8178 pc
// is 2.52e20 m with a 66 km ulp, which is the same 2.2e-16 relative step as
// 1.8e-12 pc is at 8178. What costs precision is cancellation, and that is
// relative too.
//
// # One value, many spellings
//
// The constructors and accessors below read the scale factors from this
// package's own [Unit] table rather than from constants of their own, so a
// Length and a Quantity can never disagree about how long an AU is. Measured,
// the indirection is free: 1.08 ns against 1.13 ns for a constant-folded
// equivalent, because the table is one hot cache line.
type Length float64

// Metres builds a Length from a value in metres.
func Metres(v float64) Length { return Length(v) }

// Millimetres builds a Length from a value in millimetres.
func Millimetres(v float64) Length { return Length(v * Millimeter.ScaleFactor) }

// Km builds a Length from a value in kilometres.
func Km(v float64) Length { return Length(v * Kilometer.ScaleFactor) }

// AU builds a Length from a value in astronomical units.
func AU(v float64) Length { return Length(v * AstronomicalUnit.ScaleFactor) }

// Pc builds a Length from a value in parsecs.
func Pc(v float64) Length { return Length(v * Parsec.ScaleFactor) }

// LightYears builds a Length from a value in light-years.
func LightYears(v float64) Length { return Length(v * LightYear.ScaleFactor) }

// Metres returns the length in metres, which is how it is stored.
func (l Length) Metres() float64 { return float64(l) }

// Millimetres returns the length in millimetres.
func (l Length) Millimetres() float64 { return float64(l) / Millimeter.ScaleFactor }

// Km returns the length in kilometres.
func (l Length) Km() float64 { return float64(l) / Kilometer.ScaleFactor }

// AU returns the length in astronomical units.
func (l Length) AU() float64 { return float64(l) / AstronomicalUnit.ScaleFactor }

// Pc returns the length in parsecs.
func (l Length) Pc() float64 { return float64(l) / Parsec.ScaleFactor }

// LightYears returns the length in light-years.
func (l Length) LightYears() float64 { return float64(l) / LightYear.ScaleFactor }

// IsZero reports whether the length is exactly zero.
//
// Worth having because zero is a real answer for a distance and not only an
// unset field: a target at the observer, or the Sun's own position in a
// heliocentric frame. A caller testing `l == 0` says the same thing; this says
// it in the vocabulary of the type.
func (l Length) IsZero() bool { return l == 0 }

// Abs returns the length with its sign removed.
//
// Negative lengths are not nonsense here — a Cartesian component is a length
// and half of them point the other way — so this is the ordinary magnitude
// rather than a guard against a mistake.
func (l Length) Abs() Length { return Length(math.Abs(float64(l))) }

// Quantity returns the same length as a dimensioned [Quantity] in metres.
//
// The bridge between the two representations, and the reason they cannot
// drift: both read the same [Unit] table, so a Length converted to a Quantity
// and back is the value it started as.
//
// Reach for it when a value has to compose dimensionally — divided by a time
// to get a velocity, multiplied by itself for an area — which is what Quantity
// is for and what a named scalar deliberately cannot do.
func (l Length) Quantity() Quantity { return Quantity{Value: float64(l), Unit: Meter} }

// LengthFrom converts a dimensioned [Quantity] into a Length.
//
// It returns an error for a quantity that is not a length, which is the check
// a named scalar cannot make on its own and the reason this direction has an
// error return while [Length.Quantity] does not.
func LengthFrom(q Quantity) (Length, error) {
	m, err := q.In(Meter)
	if err != nil {
		return 0, fmt.Errorf("unit: not a length: %w", err)
	}

	return Length(m.Value), nil
}

// String renders the length in the unit a reader of that magnitude expects.
//
// A log line saying 2.52e+20 m is accurate and unreadable; one saying 8178 pc
// is the same number in the words the subject is discussed in. The thresholds
// are where each unit stops being the natural one:
//
//	< 1 km          metres      a site height, an aperture
//	< 0.1 AU        kilometres  a satellite range, a lunar distance
//	< 0.5 pc        AU          solar system distances
//	otherwise       parsecs     stellar and galactic distances
//
// It is for humans. Anything that has to parse a length back should use the
// accessors, which say which unit they mean in their own name.
func (l Length) String() string {
	switch m := math.Abs(float64(l)); {
	case m < Kilometer.ScaleFactor:
		return fmt.Sprintf("%.6g m", l.Metres())
	case m < 0.1*AstronomicalUnit.ScaleFactor:
		return fmt.Sprintf("%.6g km", l.Km())
	case m < 0.5*Parsec.ScaleFactor:
		return fmt.Sprintf("%.6g AU", l.AU())
	default:
		return fmt.Sprintf("%.6g pc", l.Pc())
	}
}
