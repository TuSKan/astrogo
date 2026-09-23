package gofaext

import (
	"math"

	"github.com/hebl/gofa"
)

// The FK5 ↔ Hipparcos six-element conversions, and the reason they are not a
// two-line call through to SOFA like their siblings in gofaext.go.
//
// # What SOFA does, and where it stops
//
// iauFk52h and iauH2fk5 express the transformation as a rotation plus a spin
// applied to a barycentric pv-vector. Building that pv-vector is iauStarpv's
// job, and iauStarpv needs a distance — so it takes the parallax and inverts
// it. Two guards fire when it cannot:
//
//   - A parallax below PXMIN = 1e-7 arcsec is replaced by PXMIN, putting the
//     star on a sphere of radius 10 Mpc. Status gains 1.
//   - If the resulting speed exceeds VMAX = 0.5c, the space velocity is set to
//     **zero** — not clamped to VMAX, zeroed. Status gains 2.
//
// Both are reasonable inside iauStarpv. What is not reasonable is what happens
// next: iauFk52h and iauH2fk5 are void, so they discard that status, and the
// caller is handed a plausible-looking answer with no indication that the
// proper motion in it was invented.
//
// The damage is not subtle. A star with 150 mas/yr and no recorded parallax —
// which is most of any pre-Hipparcos proper-motion catalogue — sits at 10 Mpc
// once the distance is overridden, where 150 mas/yr is about 24c. The velocity
// is therefore zeroed, and ICRS → FK5 → ICRS returns (0, 0). Worse in the
// other direction: a star *declared at rest* in FK4 comes back from FK4 → ICRS
// → FK4 with 2.4 mas/yr in each component and 0.34 km/s of radial velocity it
// never had, because at the overridden distance FK4's fictitious proper motion
// no longer inverts. See #331 for the measured tables.
//
// # What this file does instead
//
// It observes that the distance cancels.
//
// Write the star's space velocity as its radial part plus its tangential part,
// v = (ṙ)p̂ + r(dp̂/dt). The transformation SOFA applies is
//
//	p₅ = Rᵀ·p_H            v₅ = Rᵀ·(v_H − p_H × s_H)
//
// which is linear in v and orthogonal in p, so dividing through by the
// distance r — invariant under a rotation — leaves
//
//	p̂₅ = Rᵀ·p̂_H          μ⃗₅ = Rᵀ·(μ⃗_H − p̂_H × s_H)
//
// with μ⃗ = dp̂/dt the proper motion as a tangential angular rate. The radial
// term carries through the rotation into the radial term and cancels on both
// sides, which is also why the radial velocity is returned unchanged: the spin
// contribution p̂ × s is perpendicular to p̂ and so has no radial part at all.
//
// So for a star whose distance is unknown the answer is not merely
// approximable, it is **exact**, and it never needed the distance. The
// parallax comes back unchanged for the same reason — a rotation does not move
// a star nearer or further.
//
// # Why SOFA is still used whenever it can be
//
// Because the pv route is more complete when the distance *is* known. It is
// relativistic, and it accounts for the changing light-time that distorts the
// apparent proper motion of a star with significant radial velocity (the
// Stumpff effect iauStarpv's notes describe). Neither survives the division
// above, both are real, and neither is available without a distance.
//
// The dispatch is therefore on iauStarpv's own status rather than on a
// threshold of this package's choosing: if SOFA can answer, SOFA answers, and
// the formulation here is used exactly when SOFA has told us — through the
// status its own wrappers throw away — that it cannot.

// Fk52h converts J2000.0 FK5 star data to the Hipparcos frame, which is the
// ICRS as realized by the Hipparcos catalogue.
//
// Unlike iauFk52h it does not require a usable parallax: see the commentary at
// the top of this file for what happens when SOFA's pv route cannot be taken,
// and why the answer is exact rather than approximate when it is not.
func Fk52h(r5, d5, dr5, dd5, px5, rv5 float64) (rh, dh, drh, ddh, pxh, rvh float64) {
	if starpvIsExact(r5, d5, dr5, dd5, px5, rv5) {
		gofa.Fk52h(r5, d5, dr5, dd5, px5, rv5, &rh, &dh, &drh, &ddh, &pxh, &rvh)

		return rh, dh, drh, ddh, pxh, rvh
	}

	return fk52hAngular(r5, d5, dr5, dd5, px5, rv5)
}

// fk52hAngular is [Fk52h] without the pv-vector, and so without the distance.
func fk52hAngular(r5, d5, dr5, dd5, px5, rv5 float64) (rh, dh, drh, ddh, pxh, rvh float64) {
	p, mu := starToAngular(r5, d5, dr5, dd5)

	var r5h [3][3]float64

	var s5h, wxp, vv, pOut, muOut [3]float64

	gofa.Fk5hip(&r5h, &s5h)

	// The structure below is iauFk52h's, line for line, with its pv-vector
	// replaced by the unit vector and the angular rate. Its division of the
	// spin by 365.25 is absent because that converts radians per year into the
	// radians per day its au/day velocities need; these rates are per year
	// already.

	// Orient the FK5 position into the Hipparcos system.
	gofa.Rxp(r5h, p, &pOut)

	// Apply spin to the position giving an extra angular motion component.
	gofa.Pxp(p, s5h, &wxp)

	// Add this component to the FK5 proper motion.
	gofa.Ppp(wxp, mu, &vv)

	// Orient the FK5 proper motion into the Hipparcos system.
	gofa.Rxp(r5h, vv, &muOut)

	rh, dh, drh, ddh = angularToStar(pOut, muOut)

	return rh, dh, drh, ddh, px5, rv5
}

// H2fk5 is the inverse of [Fk52h]: Hipparcos (ICRS) star data to J2000.0 FK5.
//
// It carries the same parallax-free path, and the two are exact inverses in
// that regime as well as in SOFA's.
func H2fk5(rh, dh, drh, ddh, pxh, rvh float64) (r5, d5, dr5, dd5, px5, rv5 float64) {
	if starpvIsExact(rh, dh, drh, ddh, pxh, rvh) {
		gofa.H2fk5(rh, dh, drh, ddh, pxh, rvh, &r5, &d5, &dr5, &dd5, &px5, &rv5)

		return r5, d5, dr5, dd5, px5, rv5
	}

	return h2fk5Angular(rh, dh, drh, ddh, pxh, rvh)
}

// h2fk5Angular is [H2fk5] without the pv-vector, and so without the distance.
func h2fk5Angular(rh, dh, drh, ddh, pxh, rvh float64) (r5, d5, dr5, dd5, px5, rv5 float64) {
	p, mu := starToAngular(rh, dh, drh, ddh)

	var r5h [3][3]float64

	var s5h, sh, wxp, vv, pOut, muOut [3]float64

	gofa.Fk5hip(&r5h, &s5h)

	// iauH2fk5's structure, with the same substitution as in [Fk52h].

	// Orient the spin into the Hipparcos system.
	gofa.Rxp(r5h, s5h, &sh)

	// De-orient the Hipparcos position into the FK5 system.
	gofa.Trxp(r5h, p, &pOut)

	// Apply spin to the position giving an extra angular motion component.
	gofa.Pxp(p, sh, &wxp)

	// Subtract this component from the Hipparcos proper motion.
	gofa.Pmp(mu, wxp, &vv)

	// De-orient the Hipparcos proper motion into the FK5 system.
	gofa.Trxp(r5h, vv, &muOut)

	r5, d5, dr5, dd5 = angularToStar(pOut, muOut)

	return r5, d5, dr5, dd5, pxh, rvh
}

// maxStellarSpeed bounds what a catalogue star can plausibly be doing, as a
// fraction of the speed of light.
//
// # Why SOFA's own limit is not enough
//
// iauStarpv's VMAX = 0.5c is a statement about arithmetic: past it the
// relativistic iteration stops meaning anything, so the velocity is zeroed and
// the status says so. Between a usable parallax and that ceiling there is a
// band where SOFA reports complete success and the answer is still not about a
// star. Measured at a parallax of 1e-7 arcsec — SOFA's own PXMIN, so not
// clamped — a star at rest in FK4 comes through iauFk52h with a radial
// velocity of -9046 km/s and a status of zero. The implied speed there is
// 0.43c.
//
// # Why this one is a judgement, and the only one in this package
//
// Nothing SOFA publishes separates "mathematically valid" from "not a star";
// its limits are about where its own arithmetic fails. So this is astrogo's
// number, and it is chosen from astronomy rather than from arithmetic: the
// Galaxy's escape velocity at the Sun is about 550 km/s, and the fastest
// objects a star catalogue contains are hypervelocity ejections from the
// Galactic centre at order 10^3 km/s. 0.01c is 3000 km/s — comfortably above
// anything real and fifty times below the point where SOFA gives up.
//
// It is deliberately loose. The purpose is to catch a distance that is absurd
// by orders of magnitude, not to adjudicate marginal cases: a star whose
// implied speed is a few hundred km/s keeps SOFA's answer, whatever this
// package thinks of it.
const maxStellarSpeed = 0.01

// starpvIsExact reports whether iauStarpv can build a pv-vector for this star
// without overriding anything, and whether the star it places in space is
// moving at a speed a star could be moving at.
//
// It runs iauStarpv purely for the status its callers throw away, and the pv
// it produces is discarded — the successful path below calls iauFk52h or
// iauH2fk5, which computes it again. That is a few dozen floating-point
// operations on a catalogue conversion, and it buys the property that the
// number returned in the ordinary case is SOFA's own, produced by SOFA's own
// code path, rather than something reassembled here from parts.
//
// The pv is not entirely discarded any more: its velocity is what the
// [maxStellarSpeed] test is applied to, which is the one question SOFA's
// status does not answer.
func starpvIsExact(ra, dec, dr, dd, px, rv float64) bool {
	var pv [2][3]float64

	if gofa.Starpv(ra, dec, dr, dd, px, rv, &pv) != 0 {
		return false
	}

	// gofa.Pm is the modulus of a p-vector; DC is the speed of light in the
	// au/day the pv-vector is expressed in.
	return gofa.Pm(pv[1])/gofa.DC <= maxStellarSpeed
}

// starToAngular decomposes a catalogue position and proper motion into a unit
// vector and the tangential angular-rate vector dp̂/dt, in radians per year.
//
// dr is dRA/dt — coordinate angle, SOFA's convention throughout this package —
// so the on-sky eastward rate is dr·cos(dec).
func starToAngular(ra, dec, dr, dd float64) (p, mu [3]float64) {
	sinRA, cosRA := math.Sincos(ra)
	sinDec, cosDec := math.Sincos(dec)

	p = [3]float64{cosRA * cosDec, sinRA * cosDec, sinDec}

	// The east and north unit vectors at p. Their derivation is the same as
	// the fictitious proper motion iauFk45z builds, written as a pair of basis
	// vectors rather than expanded inline.
	east := [3]float64{-sinRA, cosRA, 0}
	north := [3]float64{-cosRA * sinDec, -sinRA * sinDec, cosDec}

	onSky := dr * cosDec

	for i := range mu {
		mu[i] = onSky*east[i] + dd*north[i]
	}

	return p, mu
}

// angularToStar is the inverse of [starToAngular]: it recovers the spherical
// coordinates and the proper motion from a unit vector and its rate.
//
// # Near a pole dRA/dt is enormous, and that is the right answer
//
// The last step divides the on-sky eastward rate by cos(dec) to reach the
// coordinate rate this package speaks in, and near a pole that divisor is
// tiny: at the float64 nearest to +90° it is 6.1e-17, so a star creeping
// north-east reports some 10⁸ radians a year of right ascension.
//
// That is the geometry rather than a defect — the meridians converge, and a
// star very near a pole really does sweep right ascension very fast. It is
// also safe in composition, which is the only way this value is used: every
// caller multiplies by cos(dec) again, exactly recovering the on-sky rate that
// went in. coord's pmRACosDec is that multiplication, and
// TestTheAngularDecompositionIsExactlyInvertible is the proof.
//
// There is deliberately no guard for cos(dec) == 0. It cannot happen: π/2 is
// not representable in float64, so math.Cos never returns exactly zero, and a
// branch written for it would be dead code that reads as though it were
// protecting something. Rounding a large-but-correct rate down to zero at some
// chosen latitude would replace a right answer with a wrong one — the same
// reasoning coord's dRAdt records, where the guard *is* reachable because the
// declination there comes from a caller rather than from C2s.
func angularToStar(p, mu [3]float64) (ra, dec, dr, dd float64) {
	var lon, lat float64

	gofa.C2s(p, &lon, &lat)

	ra, dec = gofa.Anp(lon), lat

	sinRA, cosRA := math.Sincos(ra)
	sinDec, cosDec := math.Sincos(dec)

	east := [3]float64{-sinRA, cosRA, 0}
	north := [3]float64{-cosRA * sinDec, -sinRA * sinDec, cosDec}

	var onSky float64

	for i := range mu {
		onSky += mu[i] * east[i]
		dd += mu[i] * north[i]
	}

	return ra, dec, onSky / cosDec, dd
}
