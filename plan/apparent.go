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
// [coord.Context.ICRSToAltAz] does apply aberration — it is
// [coord.Context.AstrometricToObserved] and runs the whole apparent-place chain
// itself — and it is what Position feeds. Handing it a place that is already
// apparent doubles the aberration.
//
// That is not a hypothetical. The first version of this change made Position
// apparent too, on the reasoning that one object should not report two
// different places. Against the U.S. Naval Observatory's own almanac, the
// Sun's azimuth went from agreeing to 0.7" to disagreeing by 15.8" — the
// aberration, counted twice — and the USNO suite caught it the day it started
// running again (#225). Position stayed geometric; see geometricICRS.
//
// So the rule is about which pipeline the value enters, not about which object
// it describes: GeocentricVec is apparent because its consumer applies no
// aberration, Position is not because its consumer does.
func apparentVec(p eph.Provider, id eph.ID, t time.Time) (vector.Vec3, error) {
	st, err := eph.ApparentState(p, id, t)
	if err != nil {
		return vector.Vec3{}, fmt.Errorf("plan: apparent state: %w", err)
	}

	return st.Pos, nil
}

// geometricICRS is the direction half of the same targets, and it is
// deliberately *not* apparent.
//
// # Why the two accessors mean different places
//
// They are inputs to two pipelines with opposite conventions, and each wants
// the place its own consumer expects:
//
//	GeocentricVec -> coord.Context.GeocentricToObserved -> applies no aberration
//	Position      -> coord.Context.ICRSToAltAz          -> applies aberration
//
// [coord.Context.ICRSToAltAz] is [coord.Context.AstrometricToObserved]: it runs
// the full apparent-place chain itself. Handing it a place that is already
// apparent doubles the aberration, and doubling it is not a rounding error —
// measured against the U.S. Naval Observatory's own almanac, the Sun's azimuth
// went from agreeing to 0.7" to disagreeing by 15.8" the moment Position
// started returning the apparent place. That is the trap eph.ApparentState's
// own doc comment warns about, and this is what walking into it looks like.
//
// So an invariant that Position and GeocentricVec agree would be exactly wrong,
// and it was the first thing written here. The two must differ, by the
// aberration their respective consumers do or do not apply.
//
// This stays geometric rather than becoming astrometric — light time without
// aberration, which is strictly what AstrometricToObserved wants — because the
// two differ negligibly for the Sun (the 0.7" above) and Position never reaches
// that routine for a planet at all: every ephemeris-backed target here is a
// MovingBody, so observedAltAz sends it down the GeocentricVec branch. Filed
// separately rather than folded in here.
func geometricICRS(p eph.Provider, id eph.ID, t time.Time) (coord.ICRS, error) {
	vec, err := eph.Position(p, id, t)
	if err != nil {
		return coord.ICRS{}, fmt.Errorf("plan: geometric position: %w", err)
	}

	icrs, err := eph.ToICRS(vec)
	if err != nil {
		return coord.ICRS{}, fmt.Errorf("plan: geometric direction: %w", err)
	}

	return icrs, nil
}
