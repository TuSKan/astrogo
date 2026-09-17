package unit

import (
	"fmt"
	"math"
)

// Velocity units, derived from the length and time units rather than written
// down again, so a velocity and its parts cannot disagree about scale.
//
// This package declared the [Velocity] dimension but no units in it, because
// nothing needed one until the API started carrying velocities by type.
var (
	// MeterPerSecond is the SI unit of speed.
	MeterPerSecond = Meter.Div(Second)
	// KilometerPerSecond is what radial velocities are published in.
	KilometerPerSecond = Kilometer.Div(Second)
	// AstronomicalUnitPerDay is what SOFA's pv-vectors carry.
	AstronomicalUnitPerDay = AstronomicalUnit.Div(Day)
)

// Velocity is a speed, stored in meters per second.
//
// The companion to [Length], for the same reason and at the same cost: a named
// float64 is indistinguishable from the float64 it replaces, while the unit it
// is in stops being something a reader has to take on trust from a comment.
//
// Radial velocities in this library are published in km/s, SOFA's pv-vectors
// are in au/day, and SI is m/s. All three were bare float64 before this, and
// which one a given signature meant was recorded — where it was recorded at all
// — in prose beside it.
//
// Meters per second for the same reason [Length] is meters: [Second] and
// [Meter] both have ScaleFactor 1.0 here, so SI is what this package's own
// table is expressed against.
//
// # This is a speed along one axis, not a vector
//
// A named scalar cannot type a [vector.Vec3], so a three-component space
// velocity stays a Vec3 with its unit documented. What this types is every
// place a single number crosses an API boundary — a radial velocity, a speed,
// a correction to add to one — which is where the confusion actually was.
type Velocity float64

// MetersPerSec builds a Velocity from a value in meters per second.
func MetersPerSec(v float64) Velocity { return Velocity(v) }

// KmPerSec builds a Velocity from a value in kilometers per second.
func KmPerSec(v float64) Velocity { return Velocity(v * KilometerPerSecond.ScaleFactor) }

// AUPerDay builds a Velocity from a value in astronomical units per day.
func AUPerDay(v float64) Velocity { return Velocity(v * AstronomicalUnitPerDay.ScaleFactor) }

// MetersPerSec returns the velocity in meters per second, which is how it is
// stored.
func (v Velocity) MetersPerSec() float64 { return float64(v) }

// KmPerSec returns the velocity in kilometers per second.
func (v Velocity) KmPerSec() float64 { return float64(v) / KilometerPerSecond.ScaleFactor }

// AUPerDay returns the velocity in astronomical units per day.
func (v Velocity) AUPerDay() float64 { return float64(v) / AstronomicalUnitPerDay.ScaleFactor }

// IsZero reports whether the velocity is exactly zero.
//
// Zero is a real answer rather than an unset field: a star recorded at rest
// has a radial velocity of zero, and that is a different claim from having
// none — see coord.SpaceVelocity, which reports the difference with a bool.
func (v Velocity) IsZero() bool { return v == 0 }

// Abs returns the velocity with its sign removed.
//
// The sign carries meaning — negative is approaching — so this is a magnitude
// on request rather than a correction applied quietly.
func (v Velocity) Abs() Velocity { return Velocity(math.Abs(float64(v))) }

// Quantity returns the same velocity as a dimensioned [Quantity] in m/s.
func (v Velocity) Quantity() Quantity {
	return Quantity{Value: float64(v), Unit: MeterPerSecond}
}

// VelocityFrom converts a dimensioned [Quantity] into a Velocity, and reports
// a quantity that is not one.
func VelocityFrom(q Quantity) (Velocity, error) {
	ms, err := q.In(MeterPerSecond)
	if err != nil {
		return 0, fmt.Errorf("unit: not a velocity: %w", err)
	}

	return Velocity(ms.Value), nil
}

// String renders the velocity in the unit a reader of that magnitude expects.
//
// Below a kilometer per second the interesting figures are small — a solar
// apex is 18 km/s but a perspective term is meters per second — so the
// threshold is where km/s stops reading naturally.
func (v Velocity) String() string {
	if math.Abs(float64(v)) < KilometerPerSecond.ScaleFactor {
		return fmt.Sprintf("%.6g m/s", v.MetersPerSec())
	}

	return fmt.Sprintf("%.6g km/s", v.KmPerSec())
}
