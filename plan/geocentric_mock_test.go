package plan

import (
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/vector"
)

// geocentricMock is how this package's test providers answer since #803: a
// body given by its geocentric vector is located about the Earth, and the
// Earth itself at rest at the barycenter, so that an Instant composes the
// geocentric vector back and an observer at the Earth has no velocity to
// aberrate by.
func geocentricMock(id eph.ID, pos, vel vector.Vec3) (eph.State, error) {
	if id == eph.Earth {
		return eph.State{Frame: eph.FrameICRS, Center: eph.SolarSystemBarycenter}, nil
	}

	return eph.State{Pos: pos, Vel: vel, Frame: eph.FrameICRS, Center: eph.Earth}, nil
}
