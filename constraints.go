package astrogo

import (
	"math"
	"time"

	"github.com/TuSKan/astrogo/coords"
)

// Constraint evaluates observability limits binding celestial targets securely across Topocentric alignments natively.
type Constraint interface {
	// Evaluate returns true if the target meets the constraint at the given time.
	Evaluate(target Target, t time.Time, loc coords.Location) (bool, error)
}

// MinimumAltitudeConstraint asserts limits rejecting targets sinking strictly below acceptable Horizon values continuously.
type MinimumAltitudeConstraint struct {
	MinAltDegrees float64
}

// Evaluate performs explicit radian validations tracking native structural Target coordinates.
func (c *MinimumAltitudeConstraint) Evaluate(target Target, t time.Time, loc coords.Location) (bool, error) {
	alt, _, err := target.AltAz(t, loc)
	if err != nil {
		return false, err
	}

	minRad := c.MinAltDegrees * (math.Pi / 180.0)
	return alt >= minRad, nil // Must safely exceed lower boundaries exactly
}

// NighttimeConstraint mathematically rejects daylight intrusions evaluating universal Solar limits intrinsically.
type NighttimeConstraint struct {
	Sun Target // External Sun evaluator validating twilight thresholds independently
}

// Evaluate limits native evaluations strictly when local topologies traverse Astronomical Twilight structurally.
func (c *NighttimeConstraint) Evaluate(target Target, t time.Time, loc coords.Location) (bool, error) {
	sunAlt, _, err := c.Sun.AltAz(t, loc)
	if err != nil {
		return false, err
	}

	// Astronomical twilight equates natively passing negative 18 degree solar topologies
	twilightRad := -18.0 * (math.Pi / 180.0)
	return sunAlt < twilightRad, nil
}
