package constants

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/unit"
)

// Constant is a single published physical, astronomical, or geodetic
// constant: its value in SI base units, the standard uncertainty of that
// value, the unit it is expressed in, the publication it comes from, and
// whether it is exact by definition rather than measured.
//
// Value is always expressed in the unit named by Unit, and every Unit used
// in this package has ScaleFactor == 1 — so Value is simultaneously the SI
// base-unit value. Use Quantity to convert into any other compatible unit.
//
// Uncertainty is the standard (1σ, k=1) uncertainty in the same unit as
// Value, and is 0 whenever Exact is true. A value of 0 with Exact false
// means the source publishes the value without a quoted uncertainty (the
// IAU WGCCRE planetary radii are the case in point) — read Exact, never
// Uncertainty == 0, as the test of exactness.
type Constant struct {
	// Name is the constant's full name as its source publication spells
	// it, e.g. "Newtonian constant of gravitation".
	Name string
	// Symbol is the conventional symbol, e.g. "G", "c", "m_e", "α".
	Symbol string
	// Value is the constant's value, in Unit.
	Value float64
	// Uncertainty is the standard (1σ) uncertainty of Value, in Unit; 0
	// when Exact is true, or when the source quotes no uncertainty.
	Uncertainty float64
	// Unit is the unit Value and Uncertainty are expressed in.
	Unit unit.Unit
	// Reference cites the publication this value comes from, precisely
	// enough to look it up (resolution number, table number, or document
	// ID).
	Reference string
	// Exact reports whether the value is exact by definition or
	// convention — an SI defining constant, an IAU nominal/defining
	// value, or a value fixed by a geodetic standard — rather than the
	// result of a measurement.
	Exact bool
}

// Quantity returns c as a unit.Quantity, so it can take part in unit's
// dimensional arithmetic and conversions:
//
//	au, _ := constants.IAU.AstronomicalUnit.Quantity().In(unit.Kilometer)
func (c Constant) Quantity() unit.Quantity {
	return unit.New(c.Value, c.Unit)
}

// RelativeUncertainty returns Uncertainty/|Value| — the dimensionless
// relative standard uncertainty metrology tables quote alongside the
// absolute one. Returns 0 for an exact constant and for a zero Value.
func (c Constant) RelativeUncertainty() float64 {
	if c.Value == 0 {
		return 0
	}

	return c.Uncertainty / math.Abs(c.Value)
}

// String renders the constant compactly, e.g.
//
//	c = 2.99792458e+08 m/s (exact)
//	G = 6.6743e-11 ± 1.5e-15 m³/(kg·s²)
//	α = 0.0072973525643 ± 1.1e-12
//
// The unit is omitted for a dimensionless ratio (Unit.Symbol "" or "1").
func (c Constant) String() string {
	suffix := ""
	if sym := c.Unit.Symbol; sym != "" && sym != "1" {
		suffix = " " + sym
	}

	if c.Exact {
		return fmt.Sprintf("%s = %g%s (exact)", c.Symbol, c.Value, suffix)
	}

	return fmt.Sprintf("%s = %g ± %g%s", c.Symbol, c.Value, c.Uncertainty, suffix)
}

// Set is the common behavior of every constant set in this package: a
// named vintage (a publication or realization the whole set belongs to)
// and an ordered list of its members.
type Set interface {
	// Name reports the set's vintage, e.g. "CODATA 2022".
	Name() string
	// All returns every member of the set, in declaration order.
	All() []Constant
}

// Sets returns every constant set this package publishes. The order is
// stable: SI2019, CODATA2022, CODATA2018, IAU2015, WGS84, Derived, Photometric.
func Sets() []Set {
	return []Set{SI2019, CODATA2022, CODATA2018, IAU2015, WGS84, Derived, Photometric}
}
