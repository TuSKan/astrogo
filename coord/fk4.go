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
	pmRA, pmDec     angle.Angle // per Julian year; zero when hasPM is false
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

	r5, d5, dr5, dd5, px5, rv5 := gofaext.Fk425(
		c.ra.Radians(), c.dec.Radians(),
		c.pmRA.Radians(), c.pmDec.Radians(),
		c.parallax.Arcseconds(), c.rv,
	)

	return NewFK5WithProperMotion(
		angle.Rad(r5).Wrap360(), angle.Rad(d5),
		angle.Rad(dr5), angle.Rad(dd5),
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
func FK5ToFK4(c FK5, bepoch float64) FK4 {
	if !c.hasProperMotion {
		r1950, d1950, dr1950, dd1950 := gofaext.Fk54z(c.ra.Radians(), c.dec.Radians(), bepoch)

		return FK4{
			ra: angle.Rad(r1950).Wrap360(), dec: angle.Rad(d1950),
			pmRA: angle.Rad(dr1950), pmDec: angle.Rad(dd1950),
			bepoch:          bepoch,
			hasProperMotion: true,
		}
	}

	r1950, d1950, dr1950, dd1950, px1950, rv1950 := gofaext.Fk524(
		c.ra.Radians(), c.dec.Radians(),
		c.pmRA.Radians(), c.pmDec.Radians(),
		c.parallax.Arcseconds(), c.rv,
	)

	return FK4{
		ra: angle.Rad(r1950).Wrap360(), dec: angle.Rad(d1950),
		pmRA: angle.Rad(dr1950), pmDec: angle.Rad(dd1950),
		parallax:        angle.Arcsec(px1950),
		rv:              rv1950,
		bepoch:          bepoch,
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
// # Why this is not simply [FK5ToFK4] after [ICRSToFK5]
//
// [FK4ToICRS] is written as its two legs and this one is not, which looks like
// an oversight and is not. For a position with no kinematics the two routes
// genuinely differ: composing would hand the FK5 spin motion that [ICRSToFK5]
// supplies to SOFA's six-element routine, along with the zero parallax and
// zero radial velocity that are absences rather than measurements. The direct
// route uses Fk54z, which is SOFA's routine for exactly this input, and is
// what astropy's FK5 → FK4 does for a coordinate carrying no differentials.
//
// The price is that Fk54z assumes the star is at rest in FK5 while the caller
// said it is at rest in ICRS; those differ by FK5's spin, about 0.3 mas/yr in
// the *returned proper motion* and nothing at all in the returned position.
func ICRSToFK4(c ICRS, bepoch float64) FK4 {
	if c.PmRA() == angle.Zero() && c.PmDec() == angle.Zero() &&
		c.Parallax() == angle.Zero() && c.RV() == 0 {
		jd1, jd2 := ttAtJulianEpoch(J2000Epoch)
		r5, d5, _, _ := gofaext.Hfk5z(c.RA().Radians(), c.Dec().Radians(), jd1, jd2)
		r1950, d1950, dr1950, dd1950 := gofaext.Fk54z(r5, d5, bepoch)

		return FK4{
			ra: angle.Rad(r1950).Wrap360(), dec: angle.Rad(d1950),
			pmRA: angle.Rad(dr1950), pmDec: angle.Rad(dd1950),
			bepoch:          bepoch,
			hasProperMotion: true,
		}
	}

	r5, d5, dr5, dd5, px5, rv5 := gofaext.H2fk5(
		c.RA().Radians(), c.Dec().Radians(),
		c.PmRA().Radians(), c.PmDec().Radians(),
		c.Parallax().Arcseconds(), c.RV(),
	)

	r1950, d1950, dr1950, dd1950, px1950, rv1950 := gofaext.Fk524(r5, d5, dr5, dd5, px5, rv5)

	return FK4{
		ra: angle.Rad(r1950).Wrap360(), dec: angle.Rad(d1950),
		pmRA: angle.Rad(dr1950), pmDec: angle.Rad(dd1950),
		parallax:        angle.Arcsec(px1950),
		rv:              rv1950,
		bepoch:          bepoch,
		hasProperMotion: true,
	}
}
