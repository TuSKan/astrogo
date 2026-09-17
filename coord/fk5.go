package coord

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// FK5 is a position in the FK5 system, the fundamental catalogue the IAU used
// from 1984 until the ICRS replaced it in 1998.
//
// # Why it is not simply ICRS with a different name
//
// FK5 and ICRS agree to about 20 milliarcseconds, which is nothing for most
// purposes and is not nothing at all for astrometry. Two effects separate
// them:
//
//   - A **frame bias**: the FK5 axes are tilted from the ICRS axes by a fixed
//     rotation of order 20 mas, measured rather than defined, since FK5 was
//     realised from observations of stars and ICRS from VLBI of quasars.
//   - A **spin**: FK5's realisation rotates slowly against the extragalactic
//     frame, so the bias between them depends on the epoch of the position.
//     That is why [FK5ToICRS] takes an epoch and not only a direction.
//
// A catalogue that says "J2000" is usually FK5, not ICRS. The difference does
// not matter at arcsecond precision and does matter at the precision this
// library otherwise claims, which is why the two are separate types here
// rather than one type with a note in its doc comment.
//
// # What a catalogue records
//
// The same distinction [FK4] draws: a star with no recorded proper motion is
// not a star measured to have none. FK5's spin means a star at rest in FK5 has
// a real motion in ICRS, so a position-only entry goes through SOFA's
// position-only routine, which supplies that motion rather than assuming it
// away. See [NewFK5] and [NewFK5WithProperMotion].
type FK5 struct {
	ra, dec         angle.Angle
	pmRA, pmDec     angle.Angle // mu_alpha* and mu_dec per Julian year; zero when hasProperMotion is false
	parallax        angle.Angle
	rv              unit.Velocity
	jepoch          float64 // Julian epoch of observation, e.g. 2000.0
	hasProperMotion bool

	// fictitiousMotion records that the proper motion above came from the
	// frame rather than from a catalogue — here, from the slow spin of FK5
	// with respect to the ICRS, which [ICRSToFK5] hands back for a star with
	// no recorded motion of its own. See [FK4.fictitiousMotion], which carries
	// the same meaning and the fuller explanation.
	//
	// The FK5 case is smaller and was found the same way: dispatching on
	// hasProperMotion alone sent this motion down the six-element Fk52h
	// branch, which assumes J2000.0, so an ICRS → FK5 → ICRS round trip lost
	// 0.96 mas per year of offset from J2000 (#341).
	fictitiousMotion bool
}

// J2000Epoch is the Julian epoch of the FK5 equinox, and the epoch of
// observation for a catalogue that records none.
const J2000Epoch = 2000.0

// NewFK5 builds an FK5 position from a catalogue that records no proper
// motion, at the Julian epoch its positions were determined for — J2000.0
// unless the catalogue says otherwise.
//
// "No proper motion recorded" is not "proper motion is zero". FK5's frame
// spins slowly against the extragalactic one, so a star genuinely at rest in
// FK5 acquires a small motion in ICRS; SOFA supplies that through a different
// routine than the one for a star with a measured motion. Passing zeroes to
// [NewFK5WithProperMotion] asserts the star really is at rest in the inertial
// sense, which is a claim almost no catalogue makes.
func NewFK5(ra, dec angle.Angle, jepoch float64) FK5 {
	return FK5{ra: ra, dec: dec, jepoch: jepoch}
}

// NewFK5WithProperMotion builds an FK5 position from a catalogue that records
// the full six elements: proper motions per Julian year, parallax as an angle,
// and radial velocity in km/s.
//
// There is no epoch parameter, and that is not an omission: SOFA's six-element
// routines are defined for FK5 data at the catalogue equinox J2000.0, and a
// position carrying its own motion already says where it will be at any other
// date. The epoch matters only for the position-only form, which has nothing
// else to go on — see [NewFK5].
//
// Use this only for a genuinely measured proper motion. See [NewFK5].
func NewFK5WithProperMotion(ra, dec, pmRA, pmDec, parallax angle.Angle, rv unit.Velocity) FK5 {
	return FK5{
		ra: ra, dec: dec,
		pmRA: pmRA, pmDec: pmDec,
		parallax: parallax, rv: rv,
		jepoch:          J2000Epoch,
		hasProperMotion: true,
	}
}

// RA returns the FK5 right ascension.
func (c FK5) RA() angle.Angle { return c.ra }

// Dec returns the FK5 declination.
func (c FK5) Dec() angle.Angle { return c.dec }

// ProperMotion returns the proper motion per Julian year, and whether there is
// one at all.
//
// The bool separates a star carrying no motion from one carrying zero — see
// [NewFK5] for why those are different positions after conversion.
//
// As with [FK4.ProperMotion] it does not say where the motion came from: one
// returned by [ICRSToFK5] for a position-only input is the frame's own spin
// rather than the star's, and the two convert through different SOFA routines.
func (c FK5) ProperMotion() (pmRA, pmDec angle.Angle, ok bool) {
	return c.pmRA, c.pmDec, c.hasProperMotion
}

// Parallax returns the recorded annual parallax.
func (c FK5) Parallax() angle.Angle { return c.parallax }

// RV returns the recorded radial velocity.
func (c FK5) RV() unit.Velocity { return c.rv }

// Epoch returns the Julian epoch of observation.
func (c FK5) Epoch() float64 { return c.jepoch }

// String renders the position for logs and errors.
func (c FK5) String() string {
	return fmt.Sprintf("FK5 J%.1f RA %s Dec %s", c.jepoch, c.ra, c.dec)
}

// hasRecordedMotion reports whether a catalogue measured this star's proper
// motion, as opposed to there being none or it having been supplied by a frame
// conversion. It is what the six-element routes must dispatch on.
func (c FK5) hasRecordedMotion() bool { return c.hasProperMotion && !c.fictitiousMotion }

// FK5ToICRS converts an FK5 position to ICRS.
//
// ICRS is realised here by the Hipparcos frame, which is what defines it and
// what [FK4ToICRS] already routes through; astropy takes the same one.
//
// A position with no recorded proper motion goes through SOFA's position-only
// routine at the position's own epoch, because the frame spin makes the answer
// epoch-dependent — the same position quoted at 1990 and at 2020 is not the
// same ICRS direction.
//
// Kinematics survive the conversion when the catalogue recorded them: the
// returned ICRS carries proper motion, parallax and radial velocity, and
// [Context.ICRSToAltAz] will use them for rigorous space-motion propagation.
func FK5ToICRS(c FK5) ICRS {
	if !c.hasRecordedMotion() {
		jd1, jd2 := ttAtJulianEpoch(c.jepoch)
		rh, dh := gofaext.Fk5hz(c.ra.Radians(), c.dec.Radians(), jd1, jd2)

		return NewICRS(angle.Rad(rh).Wrap360(), angle.Rad(dh))
	}

	// SOFA speaks dRA/dt in and out; these types carry the catalogue's
	// on-sky rate, so each crossing converts. See [dRAdt].
	rh, dh, drh, ddh, pxh, rvh := gofaext.Fk52h(
		c.ra.Radians(), c.dec.Radians(),
		dRAdt(c.pmRA, c.dec), c.pmDec.Radians(),
		c.parallax.Arcseconds(), c.rv.KmPerSec(),
	)

	return NewICRSWithKinematics(
		angle.Rad(rh).Wrap360(), angle.Rad(dh),
		pmRACosDec(drh, angle.Rad(dh)), angle.Rad(ddh),
		angle.Arcsec(pxh), unit.KmPerSec(rvh),
	)
}

// ICRSToFK5 converts an ICRS position to FK5 at the given Julian epoch of
// observation — [J2000Epoch] for the catalogue equinox itself.
//
// The inverse of [FK5ToICRS]. A position with no kinematics attached comes
// back carrying the proper motion FK5's spin gives it, which is the honest
// answer: a star at rest in ICRS is not at rest in FK5.
//
// Which branch runs depends on whether the caller recorded kinematics, not on
// whether they happen to be zero — see ICRS.hasKinematics and #278. Both return
// the spin motion here; the difference is that the six-element route also
// carries parallax and radial velocity through.
//
// # jepoch applies to the first route only
//
// SOFA's Hfk5z takes a date, because the FK5 and ICRS axes are not merely
// rotated but spinning with respect to each other, so a star with no recorded
// motion acquires a proper motion that depends on when it is asked about.
//
// Its six-element counterpart H2fk5 takes no date. With a real proper motion in
// hand the spin is folded into the velocity instead, and the state is stated at
// J2000.0. So the six-element route answers at J2000.0 whatever jepoch says,
// and [FK5.Epoch] reports [J2000Epoch] rather than echoing the argument.
//
// This is the same limitation [ICRSToFK4] has at B1950.0, for the same reason,
// and #330 recorded it only for FK4 — it is here too. Use [PropagateEpoch] to
// move a star to another epoch, which is a different operation from a frame
// conversion and is kept separate on purpose.
func ICRSToFK5(c ICRS, jepoch float64) FK5 {
	if !c.hasKinematics {
		jd1, jd2 := ttAtJulianEpoch(jepoch)
		r5, d5, dr5, dd5 := gofaext.Hfk5z(c.RA().Radians(), c.Dec().Radians(), jd1, jd2)

		return FK5{
			ra: angle.Rad(r5).Wrap360(), dec: angle.Rad(d5),
			pmRA: pmRACosDec(dr5, angle.Rad(d5)), pmDec: angle.Rad(dd5),
			jepoch:          jepoch,
			hasProperMotion: true,
			// Hfk5z's motion comes from the FK5/ICRS spin, not from the star.
			// Saying so keeps the inverse on Fk5hz, which takes this jepoch.
			fictitiousMotion: true,
		}
	}

	r5, d5, dr5, dd5, px5, rv5 := gofaext.H2fk5(
		c.RA().Radians(), c.Dec().Radians(),
		dRAdt(c.PmRA(), c.Dec()), c.PmDec().Radians(),
		c.Parallax().Arcseconds(), c.RV().KmPerSec(),
	)

	return FK5{
		ra: angle.Rad(r5).Wrap360(), dec: angle.Rad(d5),
		pmRA: pmRACosDec(dr5, angle.Rad(d5)), pmDec: angle.Rad(dd5),
		parallax: angle.Arcsec(px5), rv: unit.KmPerSec(rv5),
		// J2000Epoch rather than jepoch: H2fk5 answers at the catalogue equinox
		// and takes no date, so echoing the caller's would label these numbers
		// with an epoch they are not at. See this function's doc comment.
		jepoch:          J2000Epoch,
		hasProperMotion: true,
	}
}

// ttAtJulianEpoch returns the two-part TT Julian date of a Julian epoch.
//
// A Julian year is 365.25 days exactly, by definition, so this is arithmetic
// rather than a calendar conversion. The offset goes into the second part so
// the first keeps the magnitude and the second keeps the precision, which is
// the whole reason SOFA takes a two-part date at all.
//
// The origin comes from [time.J2000()] rather than a written-out 2451545.0, so
// the two cannot drift apart.
func ttAtJulianEpoch(jepoch float64) (jd1, jd2 float64) {
	const daysPerJYear = 365.25

	jd1, jd2 = time.J2000().JDParts()

	return jd1, jd2 + (jepoch-J2000Epoch)*daysPerJYear
}
