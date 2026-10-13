package unit

import (
	"fmt"
	"math"
)

// Angular-rate units, derived from the angle and time units.
var (
	// RadianPerSecond is the SI unit of angular speed.
	RadianPerSecond = Radian.Div(Second)
	// DegreePerSecond is what a mount's slew rate is quoted in.
	DegreePerSecond = Degree.Div(Second)
	// RevolutionPerDay is 2π radians per day, the unit a two-line element
	// set's mean motion is published in.
	RevolutionPerDay = Unit{
		Dimension:   Radian.Div(Day).Dimension,
		ScaleFactor: 2 * math.Pi / Day.ScaleFactor,
		Name:        "revolution per day",
		Symbol:      "rev/d",
	}
)

// AngularRate is an angular speed, stored in radians per second.
//
// A mount slews in degrees per second and a satellite's mean motion is
// counted in revolutions per day, and both crossed the API as bare float64
// with the unit in a doc comment (#130). The two differ from radians per
// second by factors of 57 and 13751.
//
// There is no AngularRateFrom: a radian is dimensionless in this package's
// table, so radians per second is dimensionally a frequency, and a
// conversion from a [Quantity] could not tell one from the other.
//
// # What this does not catch
//
// An untyped constant converts silently and means radians per second:
// `SlewRate: 2` is 115°/s, not the 2°/s a mount's manual quotes, and a mean
// motion of `15.5` is 15.5 rad/s rather than fifteen and a half orbits a day.
// Write [DegreesPerSec] or [RevolutionsPerDay] around a literal.
type AngularRate float64

// RadiansPerSec builds an AngularRate from a value in radians per second.
func RadiansPerSec(v float64) AngularRate { return AngularRate(v) }

// DegreesPerSec builds an AngularRate from a value in degrees per second.
func DegreesPerSec(v float64) AngularRate { return AngularRate(v * DegreePerSecond.ScaleFactor) }

// RevolutionsPerDay builds an AngularRate from a value in revolutions per
// day.
func RevolutionsPerDay(v float64) AngularRate { return AngularRate(v * RevolutionPerDay.ScaleFactor) }

// RadiansPerSec returns the rate in radians per second, which is how it is
// stored.
func (r AngularRate) RadiansPerSec() float64 { return float64(r) }

// DegreesPerSec returns the rate in degrees per second.
func (r AngularRate) DegreesPerSec() float64 { return float64(r) / DegreePerSecond.ScaleFactor }

// RevolutionsPerDay returns the rate in revolutions per day.
func (r AngularRate) RevolutionsPerDay() float64 { return float64(r) / RevolutionPerDay.ScaleFactor }

// Quantity returns the same rate as a dimensioned [Quantity] in rad/s.
func (r AngularRate) Quantity() Quantity { return Quantity{Value: float64(r), Unit: RadianPerSecond} }

// String renders the rate in degrees per second, the unit a person reads an
// angular speed in.
func (r AngularRate) String() string { return fmt.Sprintf("%.6g°/s", r.DegreesPerSec()) }
