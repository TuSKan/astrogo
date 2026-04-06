package unit

import (
	"fmt"
	"math"
)

// Unit represents a specific measurement scale within a physical dimension.
type Unit struct {
	Dimension   Dimension // The physical dimensions of this unit.
	ScaleFactor float64   // Conversion factor to the canonical SI base unit.
	Name        string    // Full name (e.g., "meter")
	Symbol      string    // Unit symbol (e.g., "m")
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

var (
	// Length (SI)
	Meter      = Unit{Dimension: Length, ScaleFactor: 1.0, Name: "meter", Symbol: "m"}
	Kilometer  = Unit{Dimension: Length, ScaleFactor: 1000.0, Name: "kilometer", Symbol: "km"}
	Millimeter = Unit{Dimension: Length, ScaleFactor: 1e-3, Name: "millimeter", Symbol: "mm"}

	// Astronomical length
	// 1 AU = 1.495978707e11 m (IAU 2012 nominal)
	AstronomicalUnit = Unit{Dimension: Length, ScaleFactor: 1.495978707e11, Name: "astronomical unit", Symbol: "AU"}
	// 1 pc = 648000/π AU
	Parsec = Unit{Dimension: Length, ScaleFactor: 3.085677581491367e16, Name: "parsec", Symbol: "pc"}
	// 1 ly = 9.4607304725808e15 m
	LightYear = Unit{Dimension: Length, ScaleFactor: 9.4607304725808e15, Name: "light-year", Symbol: "ly"}

	// Mass
	Gram     = Unit{Dimension: Mass, ScaleFactor: 1e-3, Name: "gram", Symbol: "g"}
	Kilogram = Unit{Dimension: Mass, ScaleFactor: 1.0, Name: "kilogram", Symbol: "kg"}

	// Time
	Second = Unit{Dimension: Time, ScaleFactor: 1.0, Name: "second", Symbol: "s"}
	Minute = Unit{Dimension: Time, ScaleFactor: 60.0, Name: "minute", Symbol: "min"}
	Hour   = Unit{Dimension: Time, ScaleFactor: 3600.0, Name: "hour", Symbol: "h"}
	Day    = Unit{Dimension: Time, ScaleFactor: 86400.0, Name: "day", Symbol: "d"}

	// Angle (Dimensionless)
	Radian    = Unit{Dimension: Dimensionless, ScaleFactor: 1.0, Name: "radian", Symbol: "rad"}
	Degree    = Unit{Dimension: Dimensionless, ScaleFactor: math.Pi / 180.0, Name: "degree", Symbol: "deg"}
	Arcminute = Unit{Dimension: Dimensionless, ScaleFactor: math.Pi / (180.0 * 60.0), Name: "arcminute", Symbol: "arcmin"}
	Arcsecond = Unit{Dimension: Dimensionless, ScaleFactor: math.Pi / (180.0 * 3600.0), Name: "arcsecond", Symbol: "arcsec"}

	// Temperature
	Kelvin = Unit{Dimension: Temperature, ScaleFactor: 1.0, Name: "kelvin", Symbol: "K"}

	// Flux density (spectral)
	// 1 Jy = 1e-26 W / (m^2 Hz) = 1e-26 kg / s^2
	// Dimension: M * T^-2 (Spectral Flux Density in SI base units)
	Jansky = Unit{Dimension: SpectralFlux, ScaleFactor: 1e-26, Name: "jansky", Symbol: "Jy"}
)
