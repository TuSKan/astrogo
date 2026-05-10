package unit

import (
	"fmt"
	"math"
)

// Unit represents a specific measurement scale within a physical dimension.
type Unit struct {
	Name        string
	Symbol      string
	ScaleFactor float64
	Dimension   Dimension
}

// ── Unit Algebra ──────────────────────────────────────────────────────────────

// Compatible reports whether u and other represent the same physical dimensions.
func (u Unit) Compatible(other Unit) bool {
	return u.Dimension.Equals(other.Dimension)
}

// IncompatibleUnitError is returned when a conversion or operation is
// attempted between units with different dimensions.
type IncompatibleUnitError struct {
	From Unit
	To   Unit
}

func (e IncompatibleUnitError) Error() string {
	return fmt.Sprintf("incompatible unit conversion: from %s to %s", e.From.Symbol, e.To.Symbol)
}

// ConversionFactor returns the multiplier required to convert from u to to.
// Returns an IncompatibleUnitError if the units have different dimensions.
func (u Unit) ConversionFactor(to Unit) (float64, error) {
	if !u.Compatible(to) {
		return 0, IncompatibleUnitError{From: u, To: to}
	}

	return u.ScaleFactor / to.ScaleFactor, nil
}

// Mul returns the product of u and other (multiplies scales, adds dimensions).
func (u Unit) Mul(other Unit) Unit {
	return Unit{
		Dimension:   u.Dimension.Mul(other.Dimension),
		ScaleFactor: u.ScaleFactor * other.ScaleFactor,
		Name:        fmt.Sprintf("(%s * %s)", u.Name, other.Name),
		Symbol:      fmt.Sprintf("(%s*%s)", u.Symbol, other.Symbol),
	}
}

// Div returns the quotient of u and other (divides scales, subtracts dimensions).
func (u Unit) Div(other Unit) Unit {
	return Unit{
		Dimension:   u.Dimension.Div(other.Dimension),
		ScaleFactor: u.ScaleFactor / other.ScaleFactor,
		Name:        fmt.Sprintf("(%s / %s)", u.Name, other.Name),
		Symbol:      fmt.Sprintf("(%s/%s)", u.Symbol, other.Symbol),
	}
}

// PowInt returns u raised to the power p (multiplies scale, multiplies dimensions).
func (u Unit) PowInt(p int) Unit {
	return Unit{
		Dimension:   u.Dimension.PowInt(p),
		ScaleFactor: math.Pow(u.ScaleFactor, float64(p)),
		Name:        fmt.Sprintf("(%s^%d)", u.Name, p),
		Symbol:      fmt.Sprintf("(%s^%d)", u.Symbol, p),
	}
}

// String returns a human-readable description of the unit, e.g., "km".
func (u Unit) String() string {
	if u.Symbol != "" {
		return u.Symbol
	}

	return u.Name
}

// ── Built-in Units ─────────────────────────────────────────────────────────────

// SI and astronomical measurement units — immutable physical constants.
//
//nolint:gochecknoglobals // SI/IAU units are inherently package-level constants
var (
	// Meter is the SI base unit of length.
	Meter = Unit{Dimension: Length, ScaleFactor: 1.0, Name: "meter", Symbol: "m"}
	// Kilometer is 1000 metres.
	Kilometer = Unit{Dimension: Length, ScaleFactor: 1000.0, Name: "kilometer", Symbol: "km"}
	// Millimeter is 0.001 metres.
	Millimeter = Unit{Dimension: Length, ScaleFactor: 1e-3, Name: "millimeter", Symbol: "mm"}

	// AstronomicalUnit is 1 AU = 1.495978707e11 m (IAU 2012 nominal).
	AstronomicalUnit = Unit{Dimension: Length, ScaleFactor: 1.495978707e11, Name: "astronomical unit", Symbol: "AU"}
	// Parsec is 1 pc = 648000/π AU.
	Parsec = Unit{Dimension: Length, ScaleFactor: 3.085677581491367e16, Name: "parsec", Symbol: "pc"}
	// LightYear is 1 ly = 9.4607304725808e15 m.
	LightYear = Unit{Dimension: Length, ScaleFactor: 9.4607304725808e15, Name: "light-year", Symbol: "ly"}

	// Gram is 0.001 kilograms.
	Gram = Unit{Dimension: Mass, ScaleFactor: 1e-3, Name: "gram", Symbol: "g"}
	// Kilogram is the SI base unit of mass.
	Kilogram = Unit{Dimension: Mass, ScaleFactor: 1.0, Name: "kilogram", Symbol: "kg"}

	// Second is the SI base unit of time.
	Second = Unit{Dimension: Time, ScaleFactor: 1.0, Name: "second", Symbol: "s"}
	// Minute is 60 seconds.
	Minute = Unit{Dimension: Time, ScaleFactor: 60.0, Name: "minute", Symbol: "min"}
	// Hour is 3600 seconds.
	Hour = Unit{Dimension: Time, ScaleFactor: 3600.0, Name: "hour", Symbol: "h"}
	// Day is 86400 seconds.
	Day = Unit{Dimension: Time, ScaleFactor: 86400.0, Name: "day", Symbol: "d"}

	// Radian is the SI unit of angle (dimensionless).
	Radian = Unit{Dimension: Dimensionless, ScaleFactor: 1.0, Name: "radian", Symbol: "rad"}
	// Degree is π/180 radians.
	Degree = Unit{Dimension: Dimensionless, ScaleFactor: math.Pi / 180.0, Name: "degree", Symbol: "deg"}
	// Arcminute is 1/60 of a degree.
	Arcminute = Unit{Dimension: Dimensionless, ScaleFactor: math.Pi / (180.0 * 60.0), Name: "arcminute", Symbol: "arcmin"}
	// Arcsecond is 1/3600 of a degree.
	Arcsecond = Unit{Dimension: Dimensionless, ScaleFactor: math.Pi / (180.0 * 3600.0), Name: "arcsecond", Symbol: "arcsec"}

	// Kelvin is the SI base unit of thermodynamic temperature.
	Kelvin = Unit{Dimension: Temperature, ScaleFactor: 1.0, Name: "kelvin", Symbol: "K"}

	// Jansky is 1e-26 W/(m²·Hz), the standard unit of spectral flux density.
	Jansky = Unit{Dimension: SpectralFlux, ScaleFactor: 1e-26, Name: "jansky", Symbol: "Jy"}
)
