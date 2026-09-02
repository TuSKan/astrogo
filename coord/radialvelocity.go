package coord

import (
	"fmt"

	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/vector"
)

// BarycentricRVCorrection returns the correction, in km/s, to ADD to a
// measured (topocentric) radial velocity of target in order to refer it
// to the solar system barycenter — i.e. it removes the component of the
// observer's own barycentric motion (Earth's orbit plus the site's
// diurnal rotation) along the line of sight to target.
//
// Sign convention, stated explicitly since getting this backwards is
// the single most common defect in any RV-correction implementation:
// this is BarycentricVelocity() dotted with the unit vector FROM the
// observer TO target. If the observer is moving toward target, that dot
// product is positive, the observer's own motion blueshifts the
// measured line (makes RV_measured read too low), and this correction
// brings it back up to the true barycentric value.
//
// # Do not add this to a radial velocity
//
// Use [Context.BarycentricRadialVelocity], which composes the two
// correctly. This comment used to say
//
//	rvBarycentric = rvMeasured + ctx.BarycentricRVCorrection(target)  // WRONG
//
// and that is first-order only: redshifts multiply, so the exact form
// carries a third term rvMeasured·corr/c. Dropping it costs 4.66 m/s at
// a target velocity of 46.6 km/s and 30 m/s at 300 — larger, for such a
// target, than every relativistic term listed below put together.
//
// This function remains exported because the raw projection is a real
// quantity with real uses (its annual sinusoid, its diurnal amplitude,
// comparing against another implementation's correction). It is the
// composition that needed fixing, not the value.
//
// Accuracy: about 1 m/s, no better. gofaext.Epv00/Apco13's underlying
// ephemeris is itself accurate to a few cm/s, but this is a classical
// (non-relativistic) velocity projection — it does not implement
// gravitational redshift, light-travel-time to the barycenter, or the
// target's own proper motion/parallax effects on the projection
// geometry (the full treatment in Wright & Eastman 2014). Do not use
// this for sub-1-m/s precision-RV work.
func (ctx *Context) BarycentricRVCorrection(target ICRS) float64 {
	return ctx.BarycentricVelocity().Dot(target.ToUnitVector())
}

// lightSpeedKmPerSec is c in the units every radial velocity here is
// expressed in.
var lightSpeedKmPerSec = constants.SI2019.SpeedOfLight.Value / 1000.0

// BarycentricRadialVelocity refers a measured (topocentric) radial
// velocity of target to the solar system barycenter, in km/s.
//
// # Why this is not rvObserved plus the correction
//
// Because a radial velocity is a redshift, and redshifts compose by
// multiplication rather than addition. Two successive Doppler shifts
// give (1+z) = (1+z₁)(1+z₂), so in velocities:
//
//	rvBarycentric = rvObserved + corr + rvObserved·corr/c
//
// The third term is what adding drops. It is not a refinement: at a
// correction of 30 km/s it reaches 4.66 m/s — the size of every
// relativistic term this package documents itself as omitting, combined —
// once the target's own velocity passes 46.6 km/s, and 30 m/s for a halo
// star at 300 km/s. For the Sun-like targets most catalogs hold it is
// well under a metre per second, which is why it can sit unnoticed.
//
// It sat unnoticed here. [Context.BarycentricRVCorrection]'s own doc
// comment gave the additive form as the way to use it, and the 175-case
// Astropy fixture that validates this file could not see the error,
// because every target in it has no radial velocity at all — the term
// vanishes identically when rvObserved is zero. Astropy documents the
// same three-term formula for the same reason.
//
// The correction itself is unchanged and still classical: this fixes how
// it composes, not what it contains. See [Context.BarycentricRVCorrection]
// for the terms that remain unimplemented.
func (ctx *Context) BarycentricRadialVelocity(target ICRS, rvObserved float64) float64 {
	corr := ctx.BarycentricRVCorrection(target)

	return rvObserved + corr + rvObserved*corr/lightSpeedKmPerSec
}

// ObservedRadialVelocity returns the topocentric radial velocity, in
// km/s, an observer at ctx would measure right now for a target whose
// barycentric RV is rvBarycentric — the exact inverse of
// [Context.BarycentricRadialVelocity], and the direction almost every
// real use needs: published catalog RVs (SIMBAD's rvz_radvel, for one)
// are already barycentric, so applying BarycentricRVCorrection to one
// directly double-corrects by up to ~60 km/s peak-to-peak.
//
// Inverting rvBarycentric = rvObserved·(1 + corr/c) + corr:
//
//	rvObserved = (rvBarycentric - corr) / (1 + corr/c)
//
// which is exact rather than a series, so the round trip closes to
// floating-point precision at any radial velocity.
func (ctx *Context) ObservedRadialVelocity(target ICRS, rvBarycentric float64) float64 {
	corr := ctx.BarycentricRVCorrection(target)

	return (rvBarycentric - corr) / (1 + corr/lightSpeedKmPerSec)
}

// HeliocentricRVCorrection is [Context.BarycentricRVCorrection], but
// relative to the Sun's own barycentric motion rather than the solar
// system barycenter itself — the correction to ADD to a measured RV to
// refer it to the Sun's rest frame:
//
//	rvHeliocentric = rvMeasured + corr  // corr, err := ctx.HeliocentricRVCorrection(target)
//
// This calls gofaext.Epv00 fresh on every invocation (not cached on
// Context) — deliberately: Context.AtTime derives new epochs via a
// shallow Clone, and a cached heliocentric-velocity field would go
// silently stale across those epoch changes. Epv00 is microseconds;
// this is not a hot path. Returns ErrSofaEpv00Failed if the underlying
// SOFA computation reports a failure status.
func (ctx *Context) HeliocentricRVCorrection(target ICRS) (float64, error) {
	tdb := ctx.t.TDB()
	d1, d2 := tdb.JDParts()

	// Epv00 gives Earth's own heliocentric and barycentric state; the
	// Sun's barycentric velocity follows from
	// Sun_bary = Earth_bary - Earth_helio (both AU/day, ICRS-aligned) —
	// there is no direct "Sun's barycentric velocity" SOFA routine.
	pvh, pvb, status := gofaext.Epv00(d1, d2)
	if status < 0 {
		return 0, fmt.Errorf("%w: status %d", ErrSofaEpv00Failed, status)
	}

	auPerDayToKmPerSec := constants.IAU.AstronomicalUnit.Value / 1000.0 / constants.Derived.JulianDaySeconds.Value

	sunBarycentricVel := vector.V3(
		pvb[1][0]-pvh[1][0],
		pvb[1][1]-pvh[1][1],
		pvb[1][2]-pvh[1][2],
	).MulScalar(auPerDayToKmPerSec)

	heliocentricObserverVel := ctx.BarycentricVelocity().Sub(sunBarycentricVel)

	return heliocentricObserverVel.Dot(target.ToUnitVector()), nil
}

// TopocentricRadialVelocity returns the radial velocity, in km/s, an observer
// at ctx measures for a body whose geocentric state is posAU and velAUPerDay
// — the line-of-sight component of the body's motion relative to the
// observer, positive when the two are separating.
//
// # Why a solar-system body needs its own function
//
// [Context.BarycentricRVCorrection] and the conversions built on it exist for
// a target whose radial velocity somebody measured and wrote down: a star, a
// galaxy, anything far enough away to be a fixed direction with a catalog
// number beside it. A planet has no such number. Its radial velocity is not a
// property to be looked up but a consequence of where it and the observer are
// now, and it changes by tens of km/s over a night — which is exactly why it
// is worth computing rather than tabulating.
//
// # What is subtracted, and why it is not the barycentric velocity
//
// The state is geocentric, so the velocity to remove is the observer's own
// geocentric velocity: the site turning about Earth's axis. Earth's orbital
// motion is already absent from both sides and must not be subtracted twice.
//
// That diurnal term is omega x r for the observer's geocentric position,
// reaching 0.465 km/s at the equator and falling as the cosine of latitude.
// Small against a planet's tens of km/s, and not against the Moon's, whose
// own geocentric radial velocity stays inside about 0.06 km/s — for the Moon
// the site's rotation is the dominant term, not a correction to it.
//
// The line of sight is topocentric, from the observer rather than from the
// geocentre. For the Moon those differ by up to a degree.
func (ctx *Context) TopocentricRadialVelocity(posAU, velAUPerDay vector.Vec3) float64 {
	auKM := constants.IAU.AstronomicalUnit.Value / 1000.0
	dayS := constants.Derived.JulianDaySeconds.Value

	// Observer's geocentric position, and the velocity that position has by
	// virtue of Earth turning under it. The rotation axis is the celestial
	// pole; polar motion is tens of milliarcseconds and cannot matter to a
	// half-kilometre-per-second term.
	obsKM := ctx.ObsVec().MulScalar(auKM)
	omega := vector.V3(0, 0, constants.WGS84.AngularVelocity.Value)
	siteVel := omega.Cross(obsKM)

	bodyVel := velAUPerDay.MulScalar(auKM / dayS)

	// Topocentric line of sight, from the observer to the body.
	los := posAU.Sub(ctx.ObsVec())
	if los.Norm() == 0 {
		return 0
	}

	return bodyVel.Sub(siteVel).Dot(los.Unit())
}
