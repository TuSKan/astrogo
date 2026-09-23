package gofaext

import (
	"github.com/hebl/gofa"
)

// The FK4 ↔ FK5 six-element conversions, and why they are not the two-line
// call through to SOFA their siblings in gofaext.go are.
//
// # The same defect as fk5hip.go, reached by a different route
//
// iauFk425 and iauFk524 do not build a pv-vector and do not call iauStarpv, so
// none of the overrides described in fk5hip.go apply. They carry their own
// version of the problem instead.
//
// Both express the transformation as a constant 6x6 matrix acting on a
// *unit* pv-vector — iauFk524 literally calls iauS2pv with a radius of 1.0 —
// followed by the E-terms of aberration. The parallax never enters that
// geometry. It appears exactly twice, and only to carry the radial velocity
// in and back out again:
//
//	pxvf = px * VF        w  = rv * pxvf      (in, as a radial rate)
//	                      rv = rd / pxvf      (out, back to km/s)
//
// So the direction and the proper motion are already distance-free and
// already exact. Measured, they are bit-identical across nine decades of
// parallax for a star with no radial velocity: every difference is +0, not
// merely small. [TestFK4GeometryDoesNotDependOnDistance] pins it.
//
// # What goes wrong
//
// The last division. The 6x6 matrix and the E-terms mix the transverse motion
// into a small radial rate rd — correctly, because FK4's equinox drifts and a
// star at rest in it is genuinely moving in FK5. Recovering a physical radial
// velocity from that rate needs a distance, and dividing by an unusable
// parallax turns a frame artifact into a measurement: exactly proportional to
// 1/px, so the further the star is declared to be, the larger the velocity it
// is credited with. A star declared at rest at a parallax of 1e-9 arcsec comes
// back from FK4 -> ICRS -> FK4 with 38.9 km/s it never had. See #339 for the
// measured tables.
//
// SOFA guards this with `if (px > TINY)`, where TINY is 1e-30 — which is why a
// parallax of exactly zero is clean and 1e-9 is not. That guard is testing
// for division by zero, not for whether a distance is known.
//
// # What this file does instead
//
// Nothing to the geometry, because there is nothing to fix there. When the
// parallax does not describe a usable distance the transformation is run with
// no parallax and no radial velocity — which, since w = rv*pxvf is then zero
// either way, produces the *same* direction and proper motion to the bit — and
// the caller's own parallax and radial velocity are returned unchanged.
//
// That is the honest answer rather than an approximation of a better one: with
// no distance there is no radial velocity to recover, and a star's parallax is
// not changed by being looked at from a different equinox.
//
// The dispatch is [starpvIsExact], shared with fk5hip.go. These routines never
// call iauStarpv, so it is used here as a question rather than as a status
// check: given this position, proper motion and parallax, can a physical star
// be placed in space at all? If it cannot, the distance is not usable and the
// division above is meaningless.
//
// # The question is asked of both ends, and it has to be
//
// Asking it of the input alone catches only one direction. Going FK5 -> FK4 the
// input already carries the fictitious proper motion, so an unusable parallax
// makes an impossible star and the test fires. Going FK4 -> FK5 it does not:
// the input is a star *at rest*, with no proper motion at all and therefore no
// speed to object to, and the fictitious motion is added by the matrix
// afterwards. The impossible star is the one SOFA hands back.
//
// So both ends are tested. If either describes something that could not be a
// star, the distance behind it was not a distance, and the conversion is redone
// without one. That costs an extra iauStarpv on a catalogue conversion and
// makes the two directions agree, which a round trip closing at all requires.

// Fk425 converts B1950.0 FK4 star data to J2000.0 FK5, with the full
// six-element transformation: the E-terms of aberration are removed and FK4's
// fictitious proper motion — the drift of its non-inertial equinox — is
// subtracted. Proper motions are radians per Julian year, parallax in arcsec,
// radial velocity in km/s.
//
// Unlike iauFk425 it does not invent a radial velocity for a star whose
// parallax cannot describe a distance: see the commentary at the top of this
// file.
func Fk425(r1950, d1950, dr1950, dd1950, p1950, v1950 float64) (r2000, d2000, dr2000, dd2000, p2000, v2000 float64) {
	gofa.Fk425(r1950, d1950, dr1950, dd1950, p1950, v1950,
		&r2000, &d2000, &dr2000, &dd2000, &p2000, &v2000)

	if starpvIsExact(r1950, d1950, dr1950, dd1950, p1950, v1950) &&
		starpvIsExact(r2000, d2000, dr2000, dd2000, p2000, v2000) {
		return r2000, d2000, dr2000, dd2000, p2000, v2000
	}

	gofa.Fk425(r1950, d1950, dr1950, dd1950, 0, 0,
		&r2000, &d2000, &dr2000, &dd2000, &p2000, &v2000)

	return r2000, d2000, dr2000, dd2000, p1950, v1950
}

// Fk524 is the inverse of [Fk425]: J2000.0 FK5 to B1950.0 FK4.
//
// It carries the same distance-free path, for the same reason and with the
// same exactness.
func Fk524(r2000, d2000, dr2000, dd2000, p2000, v2000 float64) (r1950, d1950, dr1950, dd1950, p1950, v1950 float64) {
	gofa.Fk524(r2000, d2000, dr2000, dd2000, p2000, v2000,
		&r1950, &d1950, &dr1950, &dd1950, &p1950, &v1950)

	if starpvIsExact(r2000, d2000, dr2000, dd2000, p2000, v2000) &&
		starpvIsExact(r1950, d1950, dr1950, dd1950, p1950, v1950) {
		return r1950, d1950, dr1950, dd1950, p1950, v1950
	}

	gofa.Fk524(r2000, d2000, dr2000, dd2000, 0, 0,
		&r1950, &d1950, &dr1950, &dd1950, &p1950, &v1950)

	return r1950, d1950, dr1950, dd1950, p2000, v2000
}
