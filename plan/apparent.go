package plan

import (
	"fmt"

	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// apparentVec returns where a solar-system body is *seen* from the geocentre
// at t — the apparent place, with light time and annual aberration both in it.
//
// # Why this exists rather than eph.Position
//
// Every ephemeris-backed target in this package used to answer with the
// geometric vector: where the body is at t, not where it looks to be. Measured
// from Paranal across 2026, sampled every six hours with the target above 10°,
// that is the whole difference between the two through this package's own
// alt/az path:
//
//	body      samples   mean error   max error
//	Moon          632       0.71"       0.77"
//	Sun           637      20.51"      20.83"
//	Venus         605      20.88"      44.75"
//	Mars          617      28.91"      38.70"
//	Jupiter       566      15.85"      29.00"
//	Saturn        633      14.47"      27.20"
//	Neptune       643      13.45"      24.33"
//
// Both halves were missing, which is why it reaches forty arcseconds rather
// than the twenty of aberration alone. [coord.Context.GeocentricToObserved] —
// the consumer at the end of this path — adds diurnal parallax, the rotation
// into the horizon and refraction, and deliberately does not touch aberration,
// because its input is supposed to arrive apparent already. It was arriving
// geometric.
//
// # Why not the other consumer
//
// [coord.Context.ICRSToAltAz] does apply aberration, and it is what a fixed
// target goes through. Feeding an apparent place to that one would double the
// aberration and put a planet at opposition some twenty arcseconds off in the
// other direction. Which routine a target reaches is decided by whether it
// implements [MovingBody] (see observedAltAz), so the two must not be mixed:
// everything in this file is on the MovingBody side.
func apparentVec(p eph.Provider, id eph.ID, t time.Time) (vector.Vec3, error) {
	st, err := eph.ApparentState(p, id, t)
	if err != nil {
		return vector.Vec3{}, fmt.Errorf("plan: apparent state: %w", err)
	}

	return st.Pos, nil
}

// apparentICRS is apparentVec as a direction on the sky, for the Position half
// of the same targets.
//
// Position and GeocentricVec must agree about which place they mean. They feed
// different consumers — separations and details read one, alt/az reads the
// other — and a target whose altitude is apparent while its Moon separation is
// geometric is wrong in a way no single test would catch, because each answer
// is defensible on its own.
func apparentICRS(p eph.Provider, id eph.ID, t time.Time) (coord.ICRS, error) {
	vec, err := apparentVec(p, id, t)
	if err != nil {
		return coord.ICRS{}, err
	}

	icrs, err := eph.ToICRS(vec)
	if err != nil {
		return coord.ICRS{}, fmt.Errorf("plan: apparent direction: %w", err)
	}

	return icrs, nil
}
