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
// measured line (makes RV_measured read too low), and adding this
// correction brings it back up to the true barycentric value:
//
//	rvBarycentric = rvMeasured + ctx.BarycentricRVCorrection(target)
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

// ObservedRadialVelocity returns the topocentric radial velocity, in
// km/s, an observer at ctx would measure right now for a target whose
// barycentric RV is rvBarycentric — the inverse of
// BarycentricRVCorrection, and the direction almost every real use
// needs: published catalog RVs (SIMBAD's rvz_radvel, for one) are
// already barycentric, so applying BarycentricRVCorrection to one
// directly double-corrects by up to ~60 km/s peak-to-peak. Derived
// directly from BarycentricRVCorrection's own documented sign
// convention (rvBarycentric = rvMeasured + correction):
//
//	rvObserved = rvBarycentric - ctx.BarycentricRVCorrection(target)
func (ctx *Context) ObservedRadialVelocity(target ICRS, rvBarycentric float64) float64 {
	return rvBarycentric - ctx.BarycentricRVCorrection(target)
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
