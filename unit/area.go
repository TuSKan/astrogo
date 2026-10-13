package unit

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/unit/dim"
)

// Area units.
var (
	// SquareMeter is the SI unit of area.
	SquareMeter = Unit{Dimension: dim.Length.Mul(dim.Length), ScaleFactor: 1.0, Name: "square meter", Symbol: "m²"}
	// SquareCentimeter is 1e-4 square meters, the unit molecular cross
	// sections are published in.
	SquareCentimeter = Unit{Dimension: dim.Length.Mul(dim.Length), ScaleFactor: 1e-4, Name: "square centimeter", Symbol: "cm²"}
)

// Area is a surface area, stored in square meters.
//
// The companion to [Length] for the two areas that cross astrogo's API: a
// telescope's collecting area, which optics publishes in m², and a molecular
// absorption cross section, which every laboratory table publishes in cm².
// Both were bare float64 before this, with the unit in the field's name — a
// factor of 10⁴ between the two that nothing but the name stood guard over
// (#130).
//
// # What this does not catch
//
// The gap every named float64 has: an untyped constant converts silently.
// `CollectingArea: 0.5` means 0.5 m², which is right, but a cross section
// written `Sigma: 4.5e-20` is 4.5e-20 m², ten thousand times the cm² value
// the paper prints. Write [SquareCentimeters] around a literal from a table.
type Area float64

// SquareMeters builds an Area from a value in square meters.
func SquareMeters(v float64) Area { return Area(v) }

// SquareCentimeters builds an Area from a value in square centimeters.
func SquareCentimeters(v float64) Area { return Area(v * SquareCentimeter.ScaleFactor) }

// SquareMeters returns the area in square meters, which is how it is stored.
func (a Area) SquareMeters() float64 { return float64(a) }

// SquareCentimeters returns the area in square centimeters.
func (a Area) SquareCentimeters() float64 { return float64(a) / SquareCentimeter.ScaleFactor }

// Quantity returns the same area as a dimensioned [Quantity] in m².
func (a Area) Quantity() Quantity { return Quantity{Value: float64(a), Unit: SquareMeter} }

// AreaFrom converts a dimensioned [Quantity] into an Area, and reports a
// quantity that is not one.
func AreaFrom(q Quantity) (Area, error) {
	m2, err := q.In(SquareMeter)
	if err != nil {
		return 0, fmt.Errorf("unit: not an area: %w", err)
	}

	return Area(m2.Value), nil
}

// String renders the area in m² from a square centimeter up, and in cm²
// below, where a cross section lives.
func (a Area) String() string {
	if math.Abs(float64(a)) < SquareCentimeter.ScaleFactor {
		return fmt.Sprintf("%.6g cm²", a.SquareCentimeters())
	}

	return fmt.Sprintf("%.6g m²", a.SquareMeters())
}
