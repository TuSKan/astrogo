package coord

import (
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/vector"
)

// SpaceVelocity returns the target's velocity with respect to the solar system
// barycentre, in km/s, as Cartesian components on the ICRS axes.
//
// The bool reports whether the velocity could be computed at all. It is false
// for a target whose kinematics were never recorded, and false for one whose
// parallax cannot carry the conversion — see below, because that second case
// is the one worth understanding.
//
// # What it is for
//
// Proper motion is an angular rate and radial velocity is a linear one, so
// neither can be compared with the other and neither says how fast a star is
// actually going. This is the combination: the three components together, in
// one unit, on one set of axes.
//
// It is what Galactic UVW velocities, cluster membership tests, moving-group
// searches and orbit integrations all start from. astrogo had no such function
// before #335, which is why [GalactocentricFrame] carries positions only.
//
// # The frame, and what has not been removed
//
// The velocity is barycentric: the observer's own motion is gone, the Earth's
// orbit is gone, and nothing else is. In particular the **Sun's** motion is
// still in it, so two stars with identical Galactic velocities on opposite
// sides of the sky have space velocities differing by twice the solar motion.
// [LSRCorrection] is the radial-velocity analogue of removing that, and the
// full three-dimensional version is what #335 tracks.
//
// # Why a parallax is required here, when [ICRSToFK5] no longer needs one
//
// Those two look inconsistent and are not, and the difference is worth stating
// because getting it backwards produces a number that is wrong rather than
// absent.
//
// A frame conversion rotates the velocity. Rotations are linear and preserve
// length, so the distance divides out of both sides and the proper motion
// transforms on its own — which is exactly what #331's fix exploits, and why
// those conversions are exact for a star with no parallax at all.
//
// A space velocity in km/s is not a rotation. It asks how many kilometres a
// star covers in a second, and an angular rate answers that only once a
// distance says how far away the star is: the same 150 mas/yr is 7 km/s at
// 10 pc and 700 km/s at 1 kpc. There is no cancellation to exploit, so a
// target without a usable parallax has no space velocity, and this reports
// that rather than substituting a distance and returning a plausible figure.
//
// Concretely, ok is false when SOFA's iauStarpv reports that it overrode the
// distance or zeroed the speed — the same status [gofaext.Starpv] surfaces and
// the same signal #331's dispatch turns on, used here to refuse instead of to
// take another route, because here there is no other route.
func SpaceVelocity(c ICRS) (v vector.Vec3, ok bool) {
	if !c.hasKinematics {
		return vector.Zero(), false
	}

	// dRAdt because SOFA speaks the coordinate rate while this package stores
	// the catalogue's on-sky one. See [dRAdt].
	pv, status := gofaext.Starpv(
		c.ra.Radians(), c.dec.Radians(),
		dRAdt(c.pmRA, c.dec), c.pmDec.Radians(),
		c.parallax.Arcseconds(), c.rv,
	)
	if status != 0 {
		return vector.Zero(), false
	}

	// au/day to km/s.
	return vector.V3(pv[1][0], pv[1][1], pv[1][2]).MulScalar(kmPerAU / secondsPerDay), true
}

// secondsPerDay is the length of a day in seconds, exactly, by the definition
// of the Julian day SOFA's velocities are per.
const secondsPerDay = 86400.0

// SpaceSpeed returns the magnitude of [SpaceVelocity], in km/s, with the same
// meaning for the bool.
//
// It exists because the magnitude is the quantity most often wanted — a halo
// star is identified by a few hundred km/s and a disc star by a few tens —
// and because taking it from the vector by hand invites the mistake of adding
// the radial velocity to it in quadrature a second time, which double-counts
// the component already inside.
func SpaceSpeed(c ICRS) (float64, bool) {
	v, ok := SpaceVelocity(c)
	if !ok {
		return 0, false
	}

	return v.Norm(), true
}
