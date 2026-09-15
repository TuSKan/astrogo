package satellite

import (
	"fmt"
	"math"
)

// WGS-72 constants, matching the gravity model NewFromTLE hands the propagator.
//
// Copied from the backend's own getGravConst rather than from a table, because
// the point of the arithmetic below is to reproduce a branch that code takes,
// and a constant that differs in the last digit reproduces a different branch.
//
// These were WGS-84 until the propagator itself was corrected to WGS-72, which
// is what TLEs are fitted with. The cost of the mismatch in *this* file was
// small — measured across the whole Vallado suite it moves perigee by 4 m for
// every near-Earth case, and no case changes which side of the 220 km branch it
// falls on — but a comment claiming the constants match the propagator has to
// be true, or the next reader inherits a promise nobody is keeping.
const (
	earthRadiusKM = 6378.135
	muKM3S2       = 398600.8
	j2            = 0.001082616
)

// xke is sqrt(GM) in earth radii^1.5 per minute — SGP4's time and length units.
var xke = 60.0 / math.Sqrt(earthRadiusKM*earthRadiusKM*earthRadiusKM/muKM3S2)

// simplifiedDragPerigeeKM is where SGP4 switches to its simplified drag model.
//
// Not a threshold chosen here. It is the backend's own branch, spelled in its
// own units as `rp < 220.0/radiusearthkm + 1.0`, which is a perigee altitude of
// 220 km. Everything in Vallado's verification suite that astrogo could not
// reproduce lives below it — see [Satellite.Verified].
const simplifiedDragPerigeeKM = 220.0

// perigeeAltitudeKM returns the perigee altitude SGP4 itself computes, in km.
//
// # Why this is not (mu/n^2)^(1/3) * (1-e) - R
//
// Because that is the two-body value and SGP4 does not use it. A TLE's mean
// motion is a Kozai mean element, and initl converts it to the Brouwer form
// before deriving a semi-major axis — a correction worth about 1.4 km of
// perigee for a low orbit, which is the difference between reproducing the
// backend's branch and merely being near it. The sequence below is initl's,
// term for term.
//
// Confirmed against the figures Vallado wrote into his own test file: it
// returns 127.20 km where he says "perigee = 127.20", 135.75 where he says
// 135.75, and a negative perigee for the case he annotates "(perigee = -51km)".
func perigeeAltitudeKM(meanMotionRevPerDay, ecc, inclRad float64) float64 {
	// no: mean motion in radians per minute, the unit SGP4 works in.
	no := meanMotionRevPerDay * 2 * math.Pi / 1440

	eccsq := ecc * ecc
	omeosq := 1.0 - eccsq
	rteosq := math.Sqrt(omeosq)
	cosio := math.Cos(inclRad)
	cosio2 := cosio * cosio

	const twoThirds = 2.0 / 3.0

	ak := math.Pow(xke/no, twoThirds)
	d1 := 0.75 * j2 * (3.0*cosio2 - 1.0) / (rteosq * omeosq)
	del := d1 / (ak * ak)
	adel := ak * (1.0 - del*del - del*(1.0/3.0+134.0*del*del/81.0))
	del = d1 / (adel * adel)

	// The un-Kozai'd mean motion, and the semi-major axis that follows.
	ao := math.Pow(xke/(no/(1.0+del)), twoThirds)

	// rp is in earth radii; 1.0 is the surface.
	return (ao*(1.0-ecc) - 1.0) * earthRadiusKM
}

// Verified reports whether this element set sits inside the regime astrogo's
// SGP4 verification actually covers, and says why when it does not.
//
// # What the answer means
//
// astrogo measures its propagation against Vallado's reference suite (AIAA
// 2006-6753) on every run of the validation tier. Twenty-three of the thirty
// cases it can read agree to a median of 4 cm; seven do not, by 0.2 km to
// 3439 km. This reports which side of that a given element set is likely to
// fall on, so a caller is not left reading a number that looks like every
// other number.
//
// False is not "this result is wrong". It is "nothing here has been shown to
// be right", which is a different and more honest claim.
//
// # Why perigee, and only perigee
//
// Because that is what the measurement says. Below 220 km SGP4 switches to a
// simplified drag model — the backend's own `isimp` branch — and every large
// divergence in the suite lives there: the five cases that miss by more than
// 100 km have perigees of 80 to 152 km.
//
// The obvious second condition, deep space, is deliberately absent. It looked
// right and the data refuted it: of the twenty deep-space cases in the suite
// only four diverge, and all four ALSO have a perigee under 220 km, so they are
// already caught. Flagging deep space would add fifteen false alarms — every
// geostationary and Molniya case, all of which agree to centimetres — for
// nothing, and a signal that cries wolf is one people switch off.
//
// # What it costs and misses, measured
//
// Against the thirty cases: it flags ten, of which seven diverge. The three it
// flags wrongly have perigees of 182, 198 and 212 km, at the top of the band,
// and they agree to 0.27, 0.21 and 0.21 metres. It misses nothing.
//
// It used to miss one — satellite 29141, a decaying object with a 279 km
// perigee — and that turned out to say more about the propagator than about
// this predicate: 29141 diverged by 0.62 km only because the propagator was
// being run with WGS-84 constants against WGS-72 elements. With that corrected
// it agrees to 0.2 m and the blind spot went with it. Worth keeping on the
// record, because "the predicate misses slow decay" was a plausible and wrong
// explanation that survived until the real cause was found.
//
// So it is conservative near the boundary. That is stated because a caller
// deciding what to trust needs the shape of the error, not a bare boolean.
// TestVerifiedMatchesTheMeasuredDivergence asserts every number in this comment
// against the checked-in fixtures.
func (s *Satellite) Verified() (bool, string) {
	perigee := perigeeAltitudeKM(s.MeanMotion, s.ecc, s.inclRad)

	if perigee >= simplifiedDragPerigeeKM {
		return true, ""
	}

	return false, fmt.Sprintf(
		"perigee %.0f km is below %.0f km, where SGP4 uses its simplified drag model; "+
			"astrogo's verification could not reproduce Vallado's reference vectors in that "+
			"regime, by as much as 3440 km",
		perigee, simplifiedDragPerigeeKM)
}
