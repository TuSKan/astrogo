package unit

// Dimension represents the physical dimensions of a quantity using SI base unit
// exponents.
type Dimension struct {
	L     int8 // Length (meter)
	M     int8 // Mass (kilogram)
	T     int8 // Time (second)
	I     int8 // Current (ampere)
	Theta int8 // Temperature (kelvin)
	N     int8 // Quantity (mole)
	J     int8 // Luminosity (candela)
}

// ── Dimension Algebra ────────────────────────────────────────────────────────

// Equals reports whether d and other represent the same physical dimensions.
func (d Dimension) Equals(other Dimension) bool {
	return d == other
}

// Mul returns the product of dimensions d and other (adds exponents).
func (d Dimension) Mul(other Dimension) Dimension {
	return Dimension{
		L:     d.L + other.L,
		M:     d.M + other.M,
		T:     d.T + other.T,
		I:     d.I + other.I,
		Theta: d.Theta + other.Theta,
		N:     d.N + other.N,
		J:     d.J + other.J,
	}
}

// Div returns the quotient of dimensions d and other (subtracts exponents).
func (d Dimension) Div(other Dimension) Dimension {
	return Dimension{
		L:     d.L - other.L,
		M:     d.M - other.M,
		T:     d.T - other.T,
		I:     d.I - other.I,
		Theta: d.Theta - other.Theta,
		N:     d.N - other.N,
		J:     d.J - other.J,
	}
}

// PowInt returns d raised to the power p (multiplies exponents). p is
// silently truncated to int8 range ([-128,127]) to match Dimension's own
// exponent fields — physical dimension exponents in practice are always
// single digits (e.g. -2 for area⁻¹), so this is not expected to matter,
// but PowInt is not meant for arbitrarily large p.
func (d Dimension) PowInt(p int) Dimension {
	p8 := int8(p)

	return Dimension{
		L:     d.L * p8,
		M:     d.M * p8,
		T:     d.T * p8,
		I:     d.I * p8,
		Theta: d.Theta * p8,
		N:     d.N * p8,
		J:     d.J * p8,
	}
}

// ── Common Dimensions ────────────────────────────────────────────────────────

// SI base and derived dimensions — immutable physical constants.
// The Dim prefix disambiguates a dimension from a quantity type of the same
// name: [Length] and [Velocity] are values a caller passes, and a Dimension
// named Length would shadow the one they mean. Every dimension carries it,
// including the ones with no quantity type yet, so adding [Mass] or
// [Temperature] later is a new type rather than a rename of an old value.
//
// Dimensionless is the exception, and not an oversight. It cannot acquire a
// quantity type to be confused with, because a dimensionless quantity is a
// float64 and always will be — so Dimensionless would stutter for a
// disambiguation nothing needs.
var (
	Dimensionless  = Dimension{}
	DimLength      = Dimension{L: 1}
	DimMass        = Dimension{M: 1}
	DimTime        = Dimension{T: 1}
	DimCurrent     = Dimension{I: 1}
	DimTemperature = Dimension{Theta: 1}
	DimAmount      = Dimension{N: 1}
	DimLuminosity  = Dimension{J: 1}

	DimArea         = Dimension{L: 2}
	DimVolume       = Dimension{L: 3}
	DimVelocity     = Dimension{L: 1, T: -1}
	DimAccel        = Dimension{L: 1, T: -2}
	DimForce        = Dimension{L: 1, M: 1, T: -2}
	DimPressure     = Dimension{L: -1, M: 1, T: -2}
	DimEnergy       = Dimension{L: 2, M: 1, T: -2}
	DimPower        = Dimension{L: 2, M: 1, T: -3}
	DimSpectralFlux = Dimension{M: 1, T: -2} // W/(m²·Hz) base: kg·s⁻²
)
