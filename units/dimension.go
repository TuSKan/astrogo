package units

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

// PowInt returns d raised to the power p (multiplies exponents).
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

var (
	Dimensionless = Dimension{}
	Length        = Dimension{L: 1}
	Mass          = Dimension{M: 1}
	Time          = Dimension{T: 1}
	Current       = Dimension{I: 1}
	Temperature   = Dimension{Theta: 1}
	Amount        = Dimension{N: 1}
	Luminosity    = Dimension{J: 1}

	Area         = Dimension{L: 2}
	Volume        = Dimension{L: 3}
	Velocity      = Dimension{L: 1, T: -1}
	Accel         = Dimension{L: 1, T: -2}
	Force         = Dimension{L: 1, M: 1, T: -2}
	Pressure      = Dimension{L: -1, M: 1, T: -2}
	Energy        = Dimension{L: 2, M: 1, T: -2}
	Power         = Dimension{L: 2, M: 1, T: -3}
	SpectralFlux  = Dimension{M: 1, T: -2} // W/(m²·Hz) base: kg·s⁻²
)
