package unit

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/unit/dim"
)

// SquareArcsecond is one arcsecond squared, (π/648000)² sr, the unit a
// surface brightness is per. [Steradian] is declared with the radiometric
// units.
var SquareArcsecond = Unit{
	Dimension:   dim.Dimensionless,
	ScaleFactor: (math.Pi / 648000) * (math.Pi / 648000),
	Name:        "square arcsecond",
	Symbol:      "arcsec²",
}

// SolidAngle is a solid angle, stored in steradians.
//
// A detector pixel's footprint on the sky and the area of sky a map sample
// stands for were bare float64 steradians, named ...SR (#130). A solid angle
// is not an [Area] and not an angle squared by accident: the surface
// brightness it divides is per steradian or per square arcsecond, and the two
// differ by 4.25e10.
//
// There is no SolidAngleFrom. A steradian is dimensionless in this package's
// table, as a radian is, so a conversion from a [Quantity] could not refuse a
// plain number or an angle, and a check that cannot fail is worse than none.
//
// # What this does not catch
//
// An untyped constant converts silently and means steradians: a pixel written
// `PixelSolidAngle: 1` is a whole steradian, not the square arcsecond a
// camera's plate scale suggests. Write [SquareArcseconds] around a literal in
// those units.
type SolidAngle float64

// Steradians builds a SolidAngle from a value in steradians.
func Steradians(v float64) SolidAngle { return SolidAngle(v) }

// SquareArcseconds builds a SolidAngle from a value in square arcseconds.
func SquareArcseconds(v float64) SolidAngle { return SolidAngle(v * SquareArcsecond.ScaleFactor) }

// Steradians returns the solid angle in steradians, which is how it is
// stored.
func (s SolidAngle) Steradians() float64 { return float64(s) }

// SquareArcseconds returns the solid angle in square arcseconds.
func (s SolidAngle) SquareArcseconds() float64 { return float64(s) / SquareArcsecond.ScaleFactor }

// Quantity returns the same solid angle as a dimensioned [Quantity] in sr.
func (s SolidAngle) Quantity() Quantity { return Quantity{Value: float64(s), Unit: Steradian} }

// String renders the solid angle in steradians down to a millionth of one,
// and in square arcseconds below, where a pixel lives.
func (s SolidAngle) String() string {
	if math.Abs(float64(s)) < 1e-6 {
		return fmt.Sprintf("%.6g arcsec²", s.SquareArcseconds())
	}

	return fmt.Sprintf("%.6g sr", s.Steradians())
}
