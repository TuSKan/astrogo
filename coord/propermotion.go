package coord

import (
	"math"

	"github.com/TuSKan/astrogo/angle"
)

// Proper motion in right ascension comes in two flavours, and the difference
// is a factor of cos(dec) that is 1 at the equator and 0.17 at +80°.
//
// # What every catalogue publishes
//
// μα* ≡ μα·cos δ — the **angular rate on the sky** in the direction of
// increasing right ascension. Gaia's data model spells it out: "Proper motion
// in right ascension μα*≡μα cos δ of the source in ICRS ... the local tangent
// plane projection of the proper motion vector in the direction of increasing
// right ascension." SIMBAD, Hipparcos and astropy's pm_ra_cosdec are the same
// quantity.
//
// This package stores that one. It is what a caller has in hand, it is what
// [plan] prints, and it is the only one of the two whose magnitude means
// something on its own: two stars with the same μα* move across the sky at the
// same rate whatever their declination.
//
// # What SOFA wants
//
// dα/dt — the **rate of change of the coordinate**, which near a pole is
// enormous for a star that is barely moving. SOFA says so in every routine
// that takes one: "The RA proper motion is in terms of coordinate angle, not
// true angle. If the catalog uses arcseconds for both RA and Dec proper
// motions, the RA proper motion will need to be divided by cos(Dec) before
// use."
//
// # Why the conversion lives here and not at the call sites
//
// It used to live nowhere. Catalogue readers stored μα*, every consumer handed
// it to SOFA as though it were dα/dt, and the RA component of every
// Gaia- or SIMBAD-resolved proper motion came out short by cos δ — 30% at
// δ = 45°, and 83% at δ = 80°. Propagating a catalogue row twenty years moved
// it 3.47″ instead of 20″.
//
// Fixing that at each call site would have left the same trap for the next
// one. So the boundary is here: the public types carry the catalogue's
// quantity, and the two functions below are the only places the other one
// exists.

// dRAdt converts a stored proper motion — μα*, the on-sky rate — into the
// dα/dt that SOFA's routines expect.
//
// # At the poles
//
// cos δ reaches zero and dα/dt is undefined: right ascension itself has no
// meaning at a pole, so no rate of change of it does either. A star there
// moves in declination alone, and returning zero says that rather than
// dividing by zero and reporting an infinite sweep.
//
// The guard is exact equality with zero, not a tolerance. Everywhere else the
// division is what the geometry actually says — a star very near the pole
// really does sweep right ascension very fast — and rounding that off at some
// chosen latitude would replace a correct large number with a wrong small one.
func dRAdt(pmRACosDec, dec angle.Angle) float64 {
	cosDec := math.Cos(dec.Radians())
	if cosDec == 0 {
		return 0
	}

	return pmRACosDec.Radians() / cosDec
}

// pmRACosDec converts a dα/dt coming back from SOFA into the μα* this package
// stores. The inverse of [dRAdt], and exact at the poles, where it is zero for
// any finite rate.
func pmRACosDec(dRAdtRad float64, dec angle.Angle) angle.Angle {
	return angle.Rad(dRAdtRad * math.Cos(dec.Radians()))
}
