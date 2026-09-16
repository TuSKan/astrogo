package coord

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/internal/gofaext"
)

// FK4 is a position in the FK4 system, the pre-1984 fundamental catalogue
// whose equinox is B1950.0.
//
// # Why a library that reads catalogues needs this
//
// Real catalogues still carry B1950 positions — the Palomar and ESO/SERC sky
// surveys, most pre-1990 variable-star and double-star work, and every plate
// archive built on them. Reading one and treating its coordinates as J2000
// puts a target about 0.7° away, which is far outside any telescope's field
// and looks exactly like a pointing problem rather than a frame problem.
//
// # It is not a rotation
//
// FK4 to FK5 is not a change of axes. Three things happen, and two of them
// have no analogue in the Galactic or ecliptic conversions this package also
// offers:
//
//   - The equinox and equator move, which is the part that looks like a
//     rotation.
//   - The **E-terms of aberration** are removed. FK4 positions have the
//     elliptic part of annual aberration baked into them, up to 0.34″,
//     because that is how the catalogue was constructed.
//   - FK4's equinox **drifts**, so the system is not inertial. A star at rest
//     in FK4 has a real proper motion in FK5, and one with a measured FK4
//     proper motion needs that fictitious component subtracted.
//
// The third is why [FK4.ToICRS] refuses to guess. See [NewFK4] and
// [NewFK4WithProperMotion].
type FK4 struct {
	ra, dec         angle.Angle
	pmRA, pmDec     angle.Angle // mu_alpha* and mu_dec per Julian year; zero when hasPM is false
	parallax        angle.Angle
	rv              float64 // km/s
	bepoch          float64 // Besselian epoch of observation, e.g. 1950.0
	hasProperMotion bool
}

// B1950 is the Besselian epoch of the FK4 equinox, and the default epoch of
// observation for a catalogue that does not record one.
const B1950 = 1950.0

// NewFK4 builds an FK4 position from a catalogue that records no proper
// motion, at the Besselian epoch its positions were determined for — B1950.0
// unless the catalogue says otherwise, which several plate surveys do.
//
// "No proper motion recorded" is not "proper motion is zero", and the
// distinction changes the answer. A star at rest in FK4 is moving in FK5 at
// up to a few tenths of an arcsecond per century, because FK4's equinox
// drifts; SOFA supplies that fictitious motion for a position-only star
// through a different routine than the one for a star with a measured one.
// Passing zeroes to [NewFK4WithProperMotion] asserts the star really is at
// rest in the inertial sense, which is a claim almost no catalogue makes.
func NewFK4(ra, dec angle.Angle, bepoch float64) FK4 {
	return FK4{ra: ra, dec: dec, bepoch: bepoch}
}

// NewFK4WithProperMotion builds an FK4 position from a catalogue that records
// the full six elements: proper motions per Julian year, parallax as an angle,
// and radial velocity in km/s.
//
// Use this only for a genuinely measured proper motion. See [NewFK4].
func NewFK4WithProperMotion(ra, dec, pmRA, pmDec, parallax angle.Angle, rv float64) FK4 {
	return FK4{
		ra: ra, dec: dec,
		pmRA: pmRA, pmDec: pmDec,
		parallax: parallax, rv: rv,
		bepoch:          B1950,
		hasProperMotion: true,
	}
}

// RA returns the FK4 right ascension.
func (c FK4) RA() angle.Angle { return c.ra }

// Dec returns the FK4 declination.
func (c FK4) Dec() angle.Angle { return c.dec }

// ProperMotion returns the recorded proper motion per Julian year, and whether
// the catalogue recorded one at all.
//
// The bool is what separates a star with no measurement from one measured at
// zero — see [NewFK4] for why those are different positions after conversion.
func (c FK4) ProperMotion() (pmRA, pmDec angle.Angle, ok bool) {
	return c.pmRA, c.pmDec, c.hasProperMotion
}

// Parallax returns the recorded annual parallax.
func (c FK4) Parallax() angle.Angle { return c.parallax }

// RV returns the recorded radial velocity in km/s.
func (c FK4) RV() float64 { return c.rv }

// Epoch returns the Besselian epoch of observation.
func (c FK4) Epoch() float64 { return c.bepoch }

// String renders the position for logs and errors.
func (c FK4) String() string {
	return fmt.Sprintf("FK4 B%.1f RA %s Dec %s", c.bepoch, c.ra, c.dec)
}

// FK4ToFK5 converts an FK4 (B1950.0) position to FK5 (J2000.0).
//
// This is the classic B1950 → J2000 conversion, and the one most published
// coordinates were converted *with*: a plate-survey position quoted as "J2000"
// is almost always an FK5 J2000 position obtained this way, not an ICRS one.
// [FK4ToICRS] is this followed by [FK5ToICRS].
//
// A position with no recorded proper motion goes through SOFA's position-only
// routine at its own Besselian epoch, so the fictitious motion FK4's drifting
// equinox implies is accounted for rather than assumed away. The result is a
// position-only FK5, because that routine reports where the star is and not
// how fast the new frame sees it moving — [FK5ToFK4] is the direction that
// hands back a motion.
func FK4ToFK5(c FK4) FK5 {
	if !c.hasProperMotion {
		r5, d5 := gofaext.Fk45z(c.ra.Radians(), c.dec.Radians(), c.bepoch)

		return NewFK5(angle.Rad(r5).Wrap360(), angle.Rad(d5), J2000Epoch)
	}

	// SOFA's routines speak dRA/dt in and out; these types carry the
	// catalogue's on-sky rate, so each crossing converts. See [dRAdt].
	r5, d5, dr5, dd5, px5, rv5 := gofaext.Fk425(
		c.ra.Radians(), c.dec.Radians(),
		dRAdt(c.pmRA, c.dec), c.pmDec.Radians(),
		c.parallax.Arcseconds(), c.rv,
	)

	return NewFK5WithProperMotion(
		angle.Rad(r5).Wrap360(), angle.Rad(d5),
		pmRACosDec(dr5, angle.Rad(d5)), angle.Rad(dd5),
		angle.Arcsec(px5), rv5,
	)
}

// FK5ToFK4 converts an FK5 (J2000.0) position to FK4 at the given Besselian
// epoch of observation — [B1950] for the catalogue equinox itself.
//
// The inverse of [FK4ToFK5]: the E-terms of aberration are put back and the
// fictitious proper motion is restored. A position with no recorded motion
// comes back carrying the one FK4's drifting equinox gives it, which is the
// honest answer — a star at rest in FK5 is not at rest in FK4.
//
// As in [ICRSToFK4], bepoch applies to the position-only route only: a position
// with recorded kinematics comes back at B1950.0 and labelled [B1950], because
// SOFA's six-element Fk524 takes no epoch. That doc comment has the reasoning
// and names [PropagateEpoch] as the operation to use instead.
func FK5ToFK4(c FK5, bepoch float64) FK4 {
	if !c.hasProperMotion {
		r1950, d1950, dr1950, dd1950 := gofaext.Fk54z(c.ra.Radians(), c.dec.Radians(), bepoch)

		return FK4{
			ra: angle.Rad(r1950).Wrap360(), dec: angle.Rad(d1950),
			pmRA: pmRACosDec(dr1950, angle.Rad(d1950)), pmDec: angle.Rad(dd1950),
			bepoch:          bepoch,
			hasProperMotion: true,
		}
	}

	r1950, d1950, dr1950, dd1950, px1950, rv1950 := gofaext.Fk524(
		c.ra.Radians(), c.dec.Radians(),
		dRAdt(c.pmRA, c.dec), c.pmDec.Radians(),
		c.parallax.Arcseconds(), c.rv,
	)

	return FK4{
		ra: angle.Rad(r1950).Wrap360(), dec: angle.Rad(d1950),
		pmRA: pmRACosDec(dr1950, angle.Rad(d1950)), pmDec: angle.Rad(dd1950),
		parallax: angle.Arcsec(px1950),
		rv:       rv1950,
		// B1950, not bepoch — see [ICRSToFK4]'s doc comment, which records why
		// the six-element route cannot honour an epoch and what to use instead.
		bepoch:          B1950,
		hasProperMotion: true,
	}
}

// FK4ToICRS converts an FK4 (B1950.0) position to ICRS, via FK5.
//
// The route is FK4 → FK5 (J2000.0) → Hipparcos, which is what defines the
// ICRS realisation; astropy takes the same one. Both legs are the exported
// [FK4ToFK5] and [FK5ToICRS], so the intermediate J2000 position a caller may
// actually want is reachable rather than buried here.
//
// Kinematics survive the conversion when the catalogue recorded them: the
// returned ICRS carries proper motion, parallax and radial velocity, and
// [Context.ICRSToAltAz] will use them for rigorous space-motion propagation.
func FK4ToICRS(c FK4) ICRS {
	return FK5ToICRS(FK4ToFK5(c))
}

// ICRSToFK4 converts an ICRS position to FK4 at the given Besselian epoch of
// observation — [B1950] for the catalogue equinox itself.
//
// The inverse of [FK4ToICRS] and, like it, not a rotation: the E-terms of
// aberration are put back and the fictitious proper motion is restored. A
// position with no kinematics attached comes back carrying the proper motion
// FK4's drifting equinox gives it, which is the honest answer — a star at rest
// in ICRS is not at rest in FK4.
//
// # Which route depends on what the caller recorded, not on the values
//
// A position built with [NewICRS] has no kinematics recorded, and takes SOFA's
// Fk54z — the routine for exactly this input, and the matched inverse of the
// Fk45z that [FK4ToICRS] uses, which is why an archival position survives a
// round trip.
//
// A position built with [NewICRSWithKinematics] has them recorded even if they
// are zero, and takes the six-element route. Zero there is a claim: the star is
// at rest in ICRS. It is not at rest in FK5, because FK5 rotates slowly with
// respect to ICRS, so the returned FK4 proper motion carries that spin.
//
// The two were indistinguishable before #278, because the branch tested every
// kinematic field for zero rather than asking whether any had been recorded. A
// star declared at rest in ICRS therefore took the first route, which answers
// for a star at rest in FK5, and came back from ICRS → FK4 → ICRS carrying
// 0.6 to 0.9 mas/yr of proper motion it never had — while its position closed
// to 19 microarcseconds, which is what kept it invisible.
//
// # bepoch applies to the first route only
//
// The six-element route answers at B1950.0 whatever bepoch says, and the FK4 it
// returns reports [FK4.Epoch] as [B1950] rather than echoing the argument. Pass
// anything else and the value is not used.
//
// That is a real limitation and not a tidy one, so it is worth saying why it is
// the honest answer rather than a gap. SOFA's Fk54z, which the first route
// uses, carries a position to bepoch by evaluating the E-terms of aberration
// *at* bepoch and then propagating along the fictitious proper motion the frame
// gives it. Its six-element counterpart Fk524 takes no epoch at all, by design:
// with a real proper motion in hand the star's state is stated at the catalogue
// equinox and moving it is the caller's business.
//
// Propagating it here anyway would mean inventing an epoch convention SOFA does
// not define — and getting it wrong in a specific way, since the E-terms would
// be evaluated at B1950 on this route and at bepoch on the other, so the two
// branches would disagree about the same star for reasons no caller could see.
//
// Before #330 the argument was stored without being used, so ICRSToFK4(star,
// 1975) returned B1950 numbers labelled 1975. The numbers were right and the
// label was wrong, which is the worse of the two failures: a wrong label
// propagates into [FK4ToFK5]'s position-only route and into anything reading
// [FK4.Epoch].
//
// To place a star at another epoch, use [PropagateEpoch], which applies
// rigorous space motion through SOFA's Pmsafe — including parallax and the
// light-time term — and then convert. That is a different operation from a
// frame conversion, and separating them is the point.
func ICRSToFK4(c ICRS, bepoch float64) FK4 {
	if !c.hasKinematics {
		jd1, jd2 := ttAtJulianEpoch(J2000Epoch)
		r5, d5, _, _ := gofaext.Hfk5z(c.RA().Radians(), c.Dec().Radians(), jd1, jd2)
		r1950, d1950, dr1950, dd1950 := gofaext.Fk54z(r5, d5, bepoch)

		return FK4{
			ra: angle.Rad(r1950).Wrap360(), dec: angle.Rad(d1950),
			pmRA: pmRACosDec(dr1950, angle.Rad(d1950)), pmDec: angle.Rad(dd1950),
			bepoch:          bepoch,
			hasProperMotion: true,
		}
	}

	r5, d5, dr5, dd5, px5, rv5 := gofaext.H2fk5(
		c.RA().Radians(), c.Dec().Radians(),
		dRAdt(c.PmRA(), c.Dec()), c.PmDec().Radians(),
		c.Parallax().Arcseconds(), c.RV(),
	)

	// dr5 stays in SOFA's convention across this hand-off: both sides of
	// it are SOFA routines, so it is converted once, at the end.
	r1950, d1950, dr1950, dd1950, px1950, rv1950 := gofaext.Fk524(r5, d5, dr5, dd5, px5, rv5)

	return FK4{
		ra: angle.Rad(r1950).Wrap360(), dec: angle.Rad(d1950),
		pmRA: pmRACosDec(dr1950, angle.Rad(d1950)), pmDec: angle.Rad(dd1950),
		parallax: angle.Arcsec(px1950),
		rv:       rv1950,
		// B1950 rather than bepoch, and deliberately: see the note on the
		// six-element route in this function's doc comment. Fk524 answers at
		// the catalogue equinox and takes no epoch, so labelling its output
		// with the caller's would be a false claim about the numbers beside it.
		bepoch:          B1950,
		hasProperMotion: true,
	}
}
