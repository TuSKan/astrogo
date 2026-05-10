package angle

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Angle is an angular quantity stored internally as a float64 in radians.
//
// It is a value type: copy freely, pass by value, compare with ==.
// No normalization is applied on construction; use [Angle.Wrap2Pi] or
// [Angle.WrapPi] when a canonical range is needed.
type Angle float64

// Internal conversion constants — no import of constants package to keep
// this package standalone and avoid initialization-order surprises.
const (
	pi         = math.Pi
	twoPi      = 2 * pi
	rad2deg    = 180 / pi
	deg2rad    = pi / 180
	rad2arcmin = rad2deg * 60
	rad2arcsec = rad2deg * 3600
	rad2hour   = rad2deg / 15 // 1 h = 15 °
	hour2rad   = 15 * deg2rad
)

// ── Constructors ──────────────────────────────────────────────────────────────

// Rad constructs an Angle from a value already in radians.
func Rad(v float64) Angle { return Angle(v) }

// Zero returns an Angle of exactly 0 radians.
func Zero() Angle { return 0 }

// Deg constructs an Angle from a value in degrees.
func Deg(v float64) Angle { return Angle(v * deg2rad) }

// Arcmin constructs an Angle from a value in arcminutes.
func Arcmin(v float64) Angle { return Angle(v * deg2rad / 60) }

// Arcsec constructs an Angle from a value in arcseconds.
func Arcsec(v float64) Angle { return Angle(v * deg2rad / 3600) }

// Hour constructs an Angle from a value in hours (1 hour = 15 degrees).
func Hour(v float64) Angle { return Angle(v * hour2rad) }

// ── Accessors ─────────────────────────────────────────────────────────────────

// Radians returns the angle in radians.
func (a Angle) Radians() float64 { return float64(a) }

// Degrees returns the angle in degrees.
func (a Angle) Degrees() float64 { return float64(a) * rad2deg }

// Arcminutes returns the angle in arcminutes.
func (a Angle) Arcminutes() float64 { return float64(a) * rad2arcmin }

// Arcmin is an alias for [Arcminutes].
func (a Angle) Arcmin() float64 { return a.Arcminutes() }

// Arcseconds returns the angle in arcseconds.
func (a Angle) Arcseconds() float64 { return float64(a) * rad2arcsec }

// Arcsec is an alias for [Arcseconds].
func (a Angle) Arcsec() float64 { return a.Arcseconds() }

// Hours returns the angle in hours (1 hour = 15 degrees).
func (a Angle) Hours() float64 { return float64(a) * rad2hour }

// ── Trigonometry ──────────────────────────────────────────────────────────────

// Sin returns the sine of the angle.
func (a Angle) Sin() float64 { return math.Sin(float64(a)) }

// Cos returns the cosine of the angle.
func (a Angle) Cos() float64 { return math.Cos(float64(a)) }

// Tan returns the tangent of the angle.
// Returns ±Inf at ±π/2 as per IEEE 754.
func (a Angle) Tan() float64 { return math.Tan(float64(a)) }

// Asin returns the arcsine of v as an Angle.
func Asin(v float64) Angle { return Angle(math.Asin(v)) }

// Acos returns the arccosine of v as an Angle.
func Acos(v float64) Angle { return Angle(math.Acos(v)) }

// Atan returns the arctangent of v as an Angle.
func Atan(v float64) Angle { return Angle(math.Atan(v)) }

// Atan2 returns the arctangent of y/x as an Angle, using the signs of both
// arguments to determine the quadrant of the return value.
func Atan2(y, x float64) Angle { return Angle(math.Atan2(y, x)) }

// ── Normalization ─────────────────────────────────────────────────────────────

// Wrap2Pi returns an equivalent angle in [0, 2π).
//
// Examples:
//
//	Rad(3π).Wrap2Pi()  == Rad(π)
//	Rad(-π/2).Wrap2Pi() == Rad(3π/2)
//	Rad(2π).Wrap2Pi()  == Rad(0)
func (a Angle) Wrap2Pi() Angle {
	v := math.Mod(float64(a), twoPi)
	if v < 0 {
		v += twoPi
	}

	return Angle(v)
}

// WrapPi returns an equivalent angle in (-π, π].
func (a Angle) WrapPi() Angle {
	v := math.Mod(float64(a), twoPi)
	if v > pi {
		v -= twoPi
	} else if v <= -pi {
		v += twoPi
	}

	return Angle(v)
}

// Wrap360 returns an equivalent angle in [0°, 360°).
// It is an alias for [Wrap2Pi] with degree-friendly naming.
func (a Angle) Wrap360() Angle { return a.Wrap2Pi() }

// Wrap180 returns an equivalent angle in (-180°, 180°].
// It is an alias for [WrapPi] with degree-friendly naming.
func (a Angle) Wrap180() Angle { return a.WrapPi() }

// ── Arithmetic ────────────────────────────────────────────────────────────────

// Add returns a + b.
func (a Angle) Add(b Angle) Angle { return a + b }

// Sub returns a - b.
func (a Angle) Sub(b Angle) Angle { return a - b }

// MulScalar returns a * s.
func (a Angle) MulScalar(s float64) Angle { return a * Angle(s) }

// DivScalar returns a / s.
func (a Angle) DivScalar(s float64) Angle { return a / Angle(s) }

// Neg returns -a.
func (a Angle) Neg() Angle { return -a }

// Abs returns |a|.
func (a Angle) Abs() Angle { return Angle(math.Abs(float64(a))) }

// ── Formatting ────────────────────────────────────────────────────────────────

// String returns the angle in decimal degrees with 4 decimal places and "°" suffix.
func (a Angle) String() string {
	return fmt.Sprintf("%.4f°", a.Degrees())
}

// DMSString returns a formatted string in ±DD°MM'SS.S" format.
func (a Angle) DMSString(precision int) string {
	var b strings.Builder

	degVal := a.Degrees()
	if degVal < 0 {
		b.WriteByte('-')

		degVal = -degVal
	} else {
		b.WriteByte('+')
	}

	d := int64(degVal)
	rem := (degVal - float64(d)) * 60
	m := int64(rem)
	s := (rem - float64(m)) * 60

	// Handle rounding up to 60s
	if precision >= 0 {
		pow := math.Pow10(precision)
		if math.Round(s*pow)/pow >= 60 {
			s = 0

			m++
			if m >= 60 {
				m = 0
				d++
			}
		}
	}

	// Degree
	if d < 10 {
		b.WriteByte('0')
	}

	b.WriteString(strconv.FormatInt(d, 10))
	b.WriteString("°")
	// Minute
	if m < 10 {
		b.WriteByte('0')
	}

	b.WriteString(strconv.FormatInt(m, 10))
	b.WriteString("'")
	// Second
	if s < 10 {
		b.WriteByte('0')
	}

	if precision <= 0 {
		b.WriteString(strconv.FormatInt(int64(math.Round(s)), 10))
	} else {
		b.WriteString(strconv.FormatFloat(s, 'f', precision, 64))
	}

	b.WriteByte('"')

	return b.String()
}

// HMSString returns a formatted string in HHhMMmSS.Ss format.
func (a Angle) HMSString(precision int) string {
	var b strings.Builder

	hVal := a.Wrap2Pi().Hours()

	h := int64(hVal)
	rem := (hVal - float64(h)) * 60
	m := int64(rem)
	s := (rem - float64(m)) * 60

	// Handle rounding up to 60s
	if precision >= 0 {
		pow := math.Pow10(precision)
		if math.Round(s*pow)/pow >= 60 {
			s = 0

			m++
			if m >= 60 {
				m = 0

				h++
				if h >= 24 {
					h = 0
				}
			}
		}
	}

	// Hour
	if h < 10 {
		b.WriteByte('0')
	}

	b.WriteString(strconv.FormatInt(h, 10))
	b.WriteString("h")
	// Minute
	if m < 10 {
		b.WriteByte('0')
	}

	b.WriteString(strconv.FormatInt(m, 10))
	b.WriteString("m")
	// Second
	if s < 10 {
		b.WriteByte('0')
	}

	if precision <= 0 {
		b.WriteString(strconv.FormatInt(int64(math.Round(s)), 10))
	} else {
		b.WriteString(strconv.FormatFloat(s, 'f', precision, 64))
	}

	b.WriteString("s")

	return b.String()
}
