package plan

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// zenithAngle returns k's twilightThresholds altitude (plan/events.go),
// reinterpreted as an angular distance from the zenith rather than an
// altitude above the horizon (zenith angle = 90° − altitude). Terminator
// uses this as the small circle's angular radius around the subsolar
// point — e.g. CivilTwilight's -6° altitude becomes a 96° zenith angle,
// and the added GeometricTwilight/ApparentTwilight kinds (0° and -50′
// altitude respectively) become 90° and 90°50′, matching Terminator's own
// "geometric vs. apparent terminator" distinction. Unknown kinds fall back
// to the geometric 90° horizon.
func (k TwilightKind) zenithAngle() angle.Angle {
	threshold, ok := twilightThresholds[k]
	if !ok {
		return angle.Deg(90)
	}

	return angle.Deg(90 - threshold)
}

// SubsolarPoint returns the geodetic point on Earth where the Sun is
// exactly at the zenith at time t — see coord.SubPoint for the underlying
// geometry and how it differs from a nearby-body sub-point.
//
// The Sun's direction is its apparent place, with light time and annual
// aberration in it, the same one every other altitude in this package is
// computed from. The geometric place is 20 arcsec away, about 600 m here.
// It is taken to first order, from one State call rather than the four
// apparentVec iterates, which agrees to 0.04 mas for the Sun and 0.01 mas
// for the Moon (TestFirstOrderApparentPlaceAgrees).
func SubsolarPoint(p eph.Provider, t time.Time) (*coord.Geodetic, error) {
	if p == nil {
		p = eph.Default()
	}

	vec, err := firstOrderApparentVec(p, eph.Sun, t)
	if err != nil {
		return nil, fmt.Errorf("plan: subsolar point: %w", err)
	}

	geo, err := coord.SubPoint(vec, t)
	if err != nil {
		return nil, fmt.Errorf("plan: subsolar point: %w", err)
	}

	return geo, nil
}

// SublunarPoint returns the geodetic point on Earth where the Moon is
// exactly at the zenith at time t — see coord.SubPoint for the underlying
// geometry and how it differs from a nearby-body sub-point. Note this uses
// the same distance-independent direction-only definition as
// SubsolarPoint, from the Moon's apparent place; it is not the
// "sub-satellite" point, where the ellipsoid normal passes through the body.
func SublunarPoint(p eph.Provider, t time.Time) (*coord.Geodetic, error) {
	if p == nil {
		p = eph.Default()
	}

	vec, err := firstOrderApparentVec(p, eph.Moon, t)
	if err != nil {
		return nil, fmt.Errorf("plan: sublunar point: %w", err)
	}

	geo, err := coord.SubPoint(vec, t)
	if err != nil {
		return nil, fmt.Errorf("plan: sublunar point: %w", err)
	}

	return geo, nil
}

// Terminator returns n points tracing the day/night boundary of kind at
// time t — a spherical small circle of angular radius kind.zenithAngle()
// centered on the subsolar point (see coord.SmallCircle for the sampling
// geometry and its documented, negligible ellipsoidal approximation).
// Returns ErrTooFewPoints (via coord.SmallCircle) if n < 3.
func Terminator(p eph.Provider, t time.Time, kind TwilightKind, n int) ([]*coord.Geodetic, error) {
	if p == nil {
		p = eph.Default()
	}

	sub, err := SubsolarPoint(p, t)
	if err != nil {
		return nil, fmt.Errorf("plan: terminator: %w", err)
	}

	pts, err := coord.SmallCircle(sub, kind.zenithAngle(), n)
	if err != nil {
		return nil, fmt.Errorf("plan: terminator: %w", err)
	}

	return pts, nil
}
