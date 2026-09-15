package coord

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
)

// TETE is an apparent place referred to the true equator and true equinox of
// date — the system almanacs, ephemerides and telescope control systems have
// quoted "apparent RA and Dec" in for two centuries.
//
// # Why this exists next to [Apparent]
//
// [Apparent] is **not** this. It is the CIRS place: the same true equator, but
// right ascension measured from the Celestial Intermediate Origin rather than
// from the true equinox. Declination is identical in the two systems and right
// ascension is not.
//
// The gap between them is the equation of the origins, and it is not small. It
// is the precession in right ascension accumulated since J2000.0 plus the
// equation of the equinoxes, so it grows at roughly 46 arcseconds a year:
// **about 20 arcminutes in 2026**, and rising. Reading a CIRS right ascension
// as an apparent one is a pointing error three times the Moon's diameter.
//
// Both are correct answers to different questions. CIRS is what the IAU 2000/2006
// resolutions put at the centre of the transformation chain, and it is what
// [Context.ApparentToObserved] consumes. The equinox-based place is what a
// printed almanac, a FITS header written by older software, and most telescope
// control systems mean by "apparent".
//
// # What is not different
//
// Not a different equator, not a different epoch, and not a different physical
// model: aberration, light deflection, precession and nutation have all already
// been applied by the time either place exists. The two differ by the origin of
// right ascension and nothing else, which is why [Context.ApparentToTETE] shifts
// one coordinate and passes the other through untouched.
type TETE struct {
	ra  angle.Angle
	dec angle.Angle
}

// NewTETE builds an equinox-based apparent place.
func NewTETE(ra, dec angle.Angle) TETE {
	return TETE{ra: ra, dec: dec}
}

// RA returns the apparent right ascension, measured from the true equinox of
// date.
func (c TETE) RA() angle.Angle { return c.ra }

// Dec returns the apparent declination, referred to the true equator of date.
// It is the same value [Apparent] carries.
func (c TETE) Dec() angle.Angle { return c.dec }

// String renders the place for logs and errors, naming the system — because
// "RA 05h35m Dec -05°23'" is the same text a CIRS place prints and they are
// twenty arcminutes apart.
func (c TETE) String() string {
	return fmt.Sprintf("TETE RA %s Dec %s", c.ra.HMSString(2), c.dec.DMSString(1))
}

// ApparentToTETE converts a CIRS apparent place to the equinox-based one, at
// this Context's epoch.
//
// SOFA states the conversion as RA = RI − EO, the equation of the origins
// subtracted from the CIRS right ascension. The Context already has EO:
// Apco13 returns it alongside the astrometry parameters, so this costs a
// subtraction rather than a second precession-nutation evaluation.
//
// Declination is passed through, because the two systems share an equator.
//
// # Across a time step
//
// [Context.AtTime] does not recompute the equation of the origins, for the
// same reason it does not recompute precession-nutation: EO drifts at about
// 0.006 arcseconds an hour, which is the term already at the bottom of that
// method's documented error budget. A Context stepped far enough for
// precession to matter has the same problem here and the same remedy — build
// a fresh [NewContext].
func (ctx *Context) ApparentToTETE(c Apparent) TETE {
	return TETE{
		ra:  c.RA().Sub(angle.Rad(ctx.eo)).Wrap360(),
		dec: c.Dec(),
	}
}

// TETEToApparent converts an equinox-based apparent place to the CIRS one.
// The inverse of [Context.ApparentToTETE].
func (ctx *Context) TETEToApparent(c TETE) Apparent {
	return Apparent{
		ra:  c.RA().Add(angle.Rad(ctx.eo)).Wrap360(),
		dec: c.Dec(),
	}
}
