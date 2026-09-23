package coord

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// The Local Standard of Rest is the rest frame of the material at the Sun's
// distance from the Galactic center, and it is what a Galactic radial velocity
// is quoted against.
//
// # Why a barycentric radial velocity is not enough
//
// [Context.BarycentricRadialVelocity] removes the observer's own motion — the
// site's rotation and Earth's orbit — and leaves a velocity measured against
// the solar system barycenter. That is the right frame for a stellar orbit and
// the wrong one for anything Galactic, because the Sun itself is moving at
// about 18 km/s with respect to its neighbours. Two clouds with the same
// Galactic velocity, observed six months apart in opposite parts of the sky,
// have barycentric velocities that differ by up to 36 km/s for no physical
// reason.
//
// Radio spectroscopy quotes v_LSR for exactly this reason, and so do Galactic
// rotation curves, molecular cloud catalogues and H I surveys.
//
// # There is more than one of them
//
// "The" Local Standard of Rest is a convention, and the conventions disagree
// by a few km/s because they are measuring different things: a *dynamical* LSR
// is the circular orbit at the Sun's radius, derived from a stellar sample and
// a model, while a *kinematic* one is the mean motion of a chosen population.
// Each published value belongs to its survey, which is why this package names
// the convention in the call rather than picking one.
//
// # The correction is additive, deliberately
//
// [LSRCorrection] is a velocity to add. That is the convention every published
// v_LSR uses and it is what astropy's LSR frames do: the frame offset is a
// Galilean velocity added to the source's space velocity, not a second Doppler
// shift composed with the first.
//
// This differs from [Context.BarycentricRadialVelocity], which does compose
// multiplicatively, and the difference is not an oversight in either place.
// That one transforms between two observers and so is a real redshift
// composition; this one re-references a velocity to a kinematic construct that
// nobody observes from. Adding a relativistic cross term here would put astrogo
// a few m/s away from every other tool and every published number.

// LSRKind selects which Local Standard of Rest a correction refers to.
type LSRKind int

const (
	// LSRDynamical is the dynamical LSR of Schönrich, Binney & Dehnen (2010),
	// the modern default and the one astropy's LSR frame uses.
	LSRDynamical LSRKind = iota

	// LSRDelhaye is the historical dynamical LSR of Delhaye (1965), still the
	// frame a good deal of older literature is quoted in.
	LSRDelhaye

	// LSRKinematic is the kinematic Local Standard of Rest, the average of
	// the velocities of the bright stars near the Sun rather than a
	// dynamical construction. It is what radio spectroscopy means by "LSR",
	// and what a spectral line's velocity is quoted against unless the paper
	// says otherwise.
	//
	// It is published differently from the other two, which is why it arrived
	// later (#295): they are Galactic Cartesian components, and this is an
	// apex — 20 km/s toward RA 270°, Dec +30°, referred to the B1900 equinox
	// (Gordon 1975, Methods of Experimental Physics vol. 12C §6.1.5). See
	// [kinematicApexICRS] for how that direction is brought to ICRS and what
	// it costs.
	LSRKinematic
)

// String names the convention.
func (k LSRKind) String() string {
	switch k {
	case LSRDynamical:
		return "LSR (Schönrich+ 2010)"
	case LSRDelhaye:
		return "LSRD (Delhaye 1965)"
	case LSRKinematic:
		return "LSRK (Gordon 1975)"
	default:
		return fmt.Sprintf("LSRKind(%d)", int(k))
	}
}

// solarMotion returns the Sun's velocity with respect to the given Local
// Standard of Rest, in km/s, as right-handed Galactic Cartesian components:
// U toward the Galactic center, V toward Galactic rotation, W toward the north
// Galactic pole.
//
// Both dynamical conventions are the published values, entered as their
// authors give them:
//
//   - Schönrich, Binney & Dehnen (2010), MNRAS 403, 1829, table 3, whose
//     (11.1, 12.24, 7.25) is the modern determination from SDSS/Geneva-
//     Copenhagen kinematics.
//   - Delhaye (1965), in *Galactic Structure* (Blaauw & Schmidt, eds.),
//     §2.1, whose (9, 12, 7) is the classical value and whose own stated
//     apex — Galactic l = 53°, b = 25° — [TestLSRApexMatchesTheLiterature]
//     checks these components against.
//
// The second element of each is the one to look at: V is the asymmetric drift,
// and it is the component that a new survey moves. That is why the convention
// is named at the call site rather than folded into a single constant.
//
// [LSRKinematic] is not here and must not reach this: it is published as an
// apex rather than as components, and there is no (u, v, w) triple of
// Gordon's to return. [solarMotionICRS] routes it elsewhere before this is
// called.
func (k LSRKind) solarMotion() (u, v, w float64) {
	switch k {
	case LSRDynamical:
		return 11.1, 12.24, 7.25
	case LSRDelhaye:
		return 9, 12, 7
	case LSRKinematic:
		// Unreachable: solarMotionICRS sends LSRK to kinematicApexICRS before
		// this is called, because Gordon publishes an apex and not components,
		// so there is no triple of his to return.
		//
		// Listed rather than left to the default so the compiler's exhaustive
		// check covers it, and so a reader sees that the omission is deliberate
		// rather than forgotten. If the routing above were ever removed, this
		// would quietly answer with Schonrich's numbers — which is why
		// TestLSRKinematicIsDistinctFromTheDynamicalOnes asserts that LSRK's
		// apex is degrees away from both dynamical ones, and fails the moment
		// it is not.
		fallthrough
	default:
		// A kind this package does not know is a caller's mistake, and there
		// is no error return to report it through. Falling back to the modern
		// default is the least harmful answer available: a zero solar motion
		// would read as "no correction needed", which is a wrong number that
		// looks like a right one, and [LSRKind.String] already renders an
		// unknown value as itself so a log line says what happened.
		return 11.1, 12.24, 7.25
	}
}

// solarMotionICRS returns the same velocity as ICRS Cartesian components.
//
// The published values are Galactic, so the rotation is done here rather than
// a rotated copy being written down: a Cartesian triple in the ICRS is not a
// number anyone published, and copying one in would make astrogo's answer
// depend on somebody else's arithmetic instead of on the values their paper
// actually states.
func solarMotionICRS(kind LSRKind) vector.Vec3 {
	if kind == LSRKinematic {
		return kinematicApexICRS()
	}

	u, v, w := kind.solarMotion()

	speed := math.Sqrt(u*u + v*v + w*w)

	dir := GalacticToICRS(NewGalactic(
		angle.Rad(math.Atan2(v, u)).Wrap360(),
		angle.Rad(math.Asin(w/speed)),
	))

	return dir.ToUnitVector().MulScalar(speed)
}

// LSRCorrection returns the velocity, in km/s, to ADD to a barycentric radial
// velocity of target to refer it to the given Local Standard of Rest.
//
//	rvLSR := ctx.BarycentricRadialVelocity(target, rvMeasured) +
//		coord.LSRCorrection(target, coord.LSRDynamical)
//
// It depends on the direction alone — not on the epoch, and not on where the
// observer is standing. Both of those were already removed by the barycentric
// step; what is left is the Sun's own motion through its neighbourhood, which
// is the same today as it was a century ago.
//
// # Sign
//
// This is the solar motion dotted with the unit vector FROM the barycenter
// TOWARD target, so it is positive for a target near the solar apex. A star
// there is being approached by the Sun, its measured barycentric velocity
// therefore reads too low, and this correction brings it back up.
//
// The size is at most the speed of the solar motion itself — 18.0 km/s for
// [LSRDynamical], 16.6 km/s for [LSRDelhaye] — reached at the apex, zero on
// the great circle 90° from it, and negated at the antapex.
func LSRCorrection(target ICRS, kind LSRKind) unit.Velocity {
	return unit.KmPerSec(solarMotionICRS(kind).Dot(target.ToUnitVector()))
}

// LSRApex returns the direction of the Sun's motion with respect to the given
// Local Standard of Rest, and its speed.
//
// The apex is where [LSRCorrection] is largest and positive, and it is how the
// solar motion is stated in most of the literature that does not give
// Cartesian components — so this is the value to compare a paper against.
func LSRApex(kind LSRKind) (ICRS, unit.Velocity) {
	v := solarMotionICRS(kind)

	var c ICRS
	c.FromUnitVector(v)

	return c, unit.KmPerSec(v.Norm())
}

// The kinematic LSR apex, as Gordon (1975) publishes it.
//
// Methods of Experimental Physics vol. 12C §6.1.5: 20 km/s toward
// RA 270°, Dec +30°, referred to the **B1900 equinox**. The equinox is the
// part that matters and the part most citations drop.
const (
	lsrkSpeed       = 20.0  // km/s
	lsrkApexRADeg   = 270.0 // B1900 equinox
	lsrkApexDecDeg  = 30.0
	lsrkApexEquinox = 1900.0
)

// kinematicApexICRS brings Gordon's B1900 apex to ICRS.
//
// # Why this is derived rather than written down
//
// Astropy checks the converted vector in as a literal, and copying it would
// make astrogo's answer depend on somebody else's arithmetic rather than on the
// value Gordon's paper states — the same reason [solarMotionICRS] rotates the
// dynamical triples here instead of storing rotated copies.
//
// # The approximation, measured
//
// The apex is referred to the B1900 equinox and the route to ICRS runs through
// FK4, whose equinox is B1950, so the direction must be precessed across fifty
// years of FK4-era equinox. The correct model for that is Newcomb's, which
// SOFA does not provide; [gofaext.Prec76Matrix] is IAU 1976, and using it here
// is an approximation.
//
// Its size is the difference between the two precession constants, about
// 1.1 arcsec per century, and it was measured rather than assumed. Against
// Astropy's V_OFFSET_LSRK — which comes from the same Gordon apex through a
// full FK4-with-equinox implementation, and is therefore an independent
// realisation of the same definition:
//
//	astrogo  [ 0.290050549357874, -17.317263640487756, 10.001412434484337]
//	astropy  [ 0.289997068390346, -17.317264789717928, 10.001411995469470]
//
//	difference   5.35e-05 km/s   (5.4 cm/s)
//	direction    0.55 arcsec
//
// 0.55 arcsec over half a century is 1.1 arcsec per century, so the residual is
// the model difference and nothing else. 5.4 cm/s is twenty times below the
// best radial-velocity precision anyone achieves and some two thousand times
// below what spectral-line work quotes, on a convention whose own definition is
// uncertain by far more. TestLSRKMatchesAstropysRealisation pins it.
//
// If [coord.FK4] ever learns a real equinox and Newcomb precession (#295's
// option A), this becomes exact and the test tightens.
func kinematicApexICRS() vector.Vec3 {
	from1, from2 := gofaext.Epb2jd(lsrkApexEquinox)
	to1, to2 := gofaext.Epb2jd(B1950)

	ra := angle.Deg(lsrkApexRADeg).Radians()
	dec := angle.Deg(lsrkApexDecDeg).Radians()

	sra, cra := math.Sincos(ra)
	sdec, cdec := math.Sincos(dec)

	// The direction as a unit vector in the B1900 frame, precessed to B1950.
	// Built by hand rather than through an ICRS value, because it is not one:
	// calling it ICRS before the conversion would be the type saying something
	// untrue.
	p := gofaext.Rxp(
		gofaext.Prec76Matrix(from1, from2, to1, to2),
		[3]float64{cra * cdec, sra * cdec, sdec},
	)

	b1950 := NewFK4(
		angle.Rad(math.Atan2(p[1], p[0])).Wrap360(),
		angle.Rad(math.Atan2(p[2], math.Hypot(p[0], p[1]))),
		B1950,
	)

	return FK4ToICRS(b1950).ToUnitVector().MulScalar(lsrkSpeed)
}
