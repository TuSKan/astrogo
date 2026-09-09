package ephemeris

import (
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// DeflectBySun exposes the solar light-deflection step to tests in
// ephemeris_test.
//
// It exists because two tests measure what the light-time loop does, and
// [ApparentState] now applies deflection after that loop finishes. Comparing a
// hand-rolled extra pass against the finished answer would otherwise be
// comparing a deflected place with an undeflected one and reading the
// difference as failed convergence — which is exactly what happened when the
// deflection landed.
//
// Exported here rather than from the package, because a caller has no use for
// it: [ApparentState] applies it, [AstrometricState] deliberately does not,
// and there is no third thing to want.
func DeflectBySun(p Provider, pos vector.Vec3, target ID, obsTime, retardedTime time.Time) (vector.Vec3, error) {
	return deflectBySun(p, pos, target, obsTime, retardedTime)
}
