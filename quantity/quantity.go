package quantity

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/units"
)

// Quantity represents a scalar physical value with an associated unit.
type Quantity struct {
	Value float64
	Unit  units.Unit
}

// New creates a new Quantity with the given value and unit.
func New(v float64, u units.Unit) Quantity {
	return Quantity{Value: v, Unit: u}
}

// ── Conversion ────────────────────────────────────────────────────────────────

// In returns a new Quantity converted to the given unit.
// Returns an error if the units have incompatible dimensions.
func (q Quantity) In(to units.Unit) (Quantity, error) {
	factor, err := q.Unit.ConversionFactor(to)
	if err != nil {
		return Quantity{}, err
	}
	return Quantity{Value: q.Value * factor, Unit: to}, nil
}

// MustIn returns a new Quantity converted to the given unit; it panics if the
// units are incompatible. Use only when the conversion is guaranteed to be safe.
func (q Quantity) MustIn(to units.Unit) Quantity {
	res, err := q.In(to)
	if err != nil {
		panic(err)
	}
	return res
}

// ── Arithmetic ────────────────────────────────────────────────────────────────

// Add returns q + other.
// Returns an error if the quantities have different dimensions.
// The resulting quantity uses the unit of q.
func (q Quantity) Add(other Quantity) (Quantity, error) {
	converted, err := other.In(q.Unit)
	if err != nil {
		return Quantity{}, err
	}
	return Quantity{Value: q.Value + converted.Value, Unit: q.Unit}, nil
}

// Sub returns q - other.
// Returns an error if the quantities have different dimensions.
// The resulting quantity uses the unit of q.
func (q Quantity) Sub(other Quantity) (Quantity, error) {
	converted, err := other.In(q.Unit)
	if err != nil {
		return Quantity{}, err
	}
	return Quantity{Value: q.Value - converted.Value, Unit: q.Unit}, nil
}

// Mul returns the product q * other.
// The resulting quantity has a derived unit (dim(q) + dim(other)).
func (q Quantity) Mul(other Quantity) Quantity {
	return Quantity{
		Value: q.Value * other.Value,
		Unit:  q.Unit.Mul(other.Unit),
	}
}

// Div returns the quotient q / other.
// The resulting quantity has a derived unit (dim(q) - dim(other)).
func (q Quantity) Div(other Quantity) Quantity {
	return Quantity{
		Value: q.Value / other.Value,
		Unit:  q.Unit.Div(other.Unit),
	}
}

// ── Formatting ────────────────────────────────────────────────────────────────

// String returns a human-readable representation of the quantity,
// e.g., "1.23 km".
func (q Quantity) String() string {
	return fmt.Sprintf("%g %s", q.Value, q.Unit.Symbol)
}

// Equals reports whether q and other represent the same physical value.
func (q Quantity) Equals(other Quantity) bool {
	if !q.Unit.Compatible(other.Unit) {
		return false
	}
	// Convert both to SI base for exact comparison
	v1 := q.Value * q.Unit.ScaleFactor
	v2 := other.Value * other.Unit.ScaleFactor
	// Tight tolerance for float comparison
	return math.Abs(v1-v2) < 1e-15*math.Max(math.Abs(v1), math.Abs(v2))
}
