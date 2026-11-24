// Package posmatch holds the one positional rule that catalog's Resolver and
// catalog/xmatch's Match share: where a Target is once its own motion is
// attached, how far apart two such positions are at a common epoch, and
// whether they may be one object.
//
// It is one package because the rule used to be written twice. #627 found
// that neither copy ever applied a proper motion; #629 fixed the Resolver's
// copy, and catalog/xmatch kept the bug until #726.
package posmatch

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/internal/gofaext"
)

// Coord returns t's position carrying t's own proper motion, parallax and
// radial velocity.
//
// Every provider keeps those in the Target's fields and builds Coord from RA
// and Dec alone, and coord.PropagateEpoch reads them from the ICRS value, so
// propagating t.Coord itself moves nothing. Until #627 no position in the
// Resolver was ever moved, and a star faster than about 0.125″/yr never
// matched its own Gaia row, 16 years from SIMBAD's J2000.
func Coord(t resolve.Target) coord.ICRS {
	if t.PmRA == 0 && t.PmDec == 0 && t.Parallax == 0 && t.RadialVelocity == 0 {
		return t.Coord
	}

	return coord.NewICRSWithKinematics(t.Coord.RA(), t.Coord.Dec(), t.PmRA, t.PmDec, t.Parallax, t.RadialVelocity, t.Epoch)
}

// Separation returns the angle between a, given at aEpoch, and b, given at
// bEpoch, with both at one epoch.
//
// When both carry a proper motion that epoch is J2000. When only one does, it
// is the other's own epoch, since a position without a motion cannot be
// moved: a catalog row that publishes none is compared where the star was
// when it was measured. When neither does, the positions are compared as
// given. A moving position is moved from its own [coord.ICRS.Epoch], as
// [Coord] sets it, so aEpoch and bEpoch matter only for one that does not
// move.
func Separation(a coord.ICRS, aEpoch epoch.Time, b coord.ICRS, bEpoch epoch.Time) (angle.Angle, error) {
	a, _, b, _, err := atCommonEpoch(a, aEpoch, b, bEpoch)
	if err != nil {
		return 0, err
	}

	return coord.Separation(a, b), nil
}

// atCommonEpoch moves a and b to the epoch [Separation] compares them at, and
// returns the epoch each was compared at: one epoch for both, unless neither
// moves and each is compared where it was given.
func atCommonEpoch(a coord.ICRS, aEpoch epoch.Time, b coord.ICRS, bEpoch epoch.Time) (
	coord.ICRS, epoch.Time, coord.ICRS, epoch.Time, error,
) {
	if aEpoch.IsZero() {
		aEpoch = epoch.J2000()
	}

	if bEpoch.IsZero() {
		bEpoch = epoch.J2000()
	}

	switch {
	case moving(a) && moving(b):
		aEpoch, bEpoch = epoch.J2000(), epoch.J2000()
	case moving(a):
		aEpoch = bEpoch
	case moving(b):
		bEpoch = aEpoch
	}

	var err error

	if moving(a) {
		if a, err = coord.PropagateEpoch(a, aEpoch); err != nil {
			return a, aEpoch, b, bEpoch, fmt.Errorf("catalog: %w", err)
		}
	}

	if moving(b) {
		if b, err = coord.PropagateEpoch(b, bEpoch); err != nil {
			return a, aEpoch, b, bEpoch, fmt.Errorf("catalog: %w", err)
		}
	}

	return a, aEpoch, b, bEpoch, nil
}

// moving reports whether c carries a proper motion to propagate it by.
func moving(c coord.ICRS) bool {
	return c.PmRA() != 0 || c.PmDec() != 0
}

// maxD2 is the largest D² of two positions that may be one object: the
// 99.73% point of a χ² with two degrees of freedom, as 3σ is of a normal
// error in one.
var maxD2 = -2 * math.Log(1-0.9973)

// Evidence is how two positions compared.
type Evidence struct {
	// Separation is their angular separation at the comparison epoch.
	Separation angle.Angle
	// D2 is Δᵀ(C₁+C₂)⁻¹Δ, where HasD2 says it was computed.
	D2    float64
	HasD2 bool
}

// Compare compares a's and b's positions at the epoch [Separation] compares
// them at, and reports whether they may be one object.
//
// Where both publish their position errors, and the motion's errors wherever
// moving those errors to that epoch needs them, it computes
// D² = Δᵀ(C₁+C₂)⁻¹Δ: Δ is b's offset from a on the plane tangent at a, and C₁
// and C₂ are the two positions' covariances at that epoch. They may be one
// object when D² is at most the 99.73% point of a χ² with two degrees of
// freedom. Otherwise they may be one object when they are closer together
// than resolution, which they never are when resolution is zero or below:
// without stated errors, only a caller's resolution says how close is one
// object.
//
// D² weighs the offset by the errors the catalogs state and by nothing else.
// Two catalogs that move a star between epochs by different models disagree
// by more than those errors. SIMBAD moves a Gaia position to J2000.0 without
// a radial velocity: read at face value, 61 Cyg A's lands 26 mas from its
// Gaia row moved there with its own, against errors under 1 mas.
// catalog/simbad reports such a star at Gaia's epoch instead (#1248), but a
// pair that disagrees this way is incompatible by position and is one star
// by an identifier.
func Compare(a, b resolve.Target, resolution angle.Angle) (Evidence, bool, error) {
	pa, ta, pb, tb, err := atCommonEpoch(Coord(a), a.Epoch, Coord(b), b.Epoch)
	if err != nil {
		return Evidence{}, false, err
	}

	ev := Evidence{Separation: coord.Separation(pa, pb)}

	ca, okA := covarianceAt(a, ta)
	cb, okB := covarianceAt(b, tb)

	if okA && okB {
		if d2, ok := mahalanobis(pa, pb, covariance{ca.xx + cb.xx, ca.xy + cb.xy, ca.yy + cb.yy}); ok {
			ev.D2, ev.HasD2 = d2, true

			return ev, d2 <= maxD2, nil
		}
	}

	return ev, resolution > 0 && ev.Separation < resolution, nil
}

// covariance is a position's covariance over (α*, δ), in square radians.
type covariance struct{ xx, xy, yy float64 }

// covarianceAt is t's position covariance at tc. It is false when t
// publishes no position error, or publishes its errors at another epoch than
// tc and no motion error to move them by.
//
// The errors move as the linear position p(t) = p₀ + μ·Δt does:
// C(t) = Cpp + Δt·(Cpμ + Cμp) + Δt²·Cμμ.
func covarianceAt(t resolve.Target, tc epoch.Time) (covariance, bool) {
	e := t.AstrometricError
	if e.RA <= 0 || e.Dec <= 0 {
		return covariance{}, false
	}

	sx, sy := e.RA.Radians(), e.Dec.Radians()
	c := covariance{xx: sx * sx, xy: e.RADec * sx * sy, yy: sy * sy}

	dt := tc.Sub(errorEpoch(t)).JulianYears()
	if dt == 0 {
		return c, true
	}

	if e.PmRA <= 0 || e.PmDec <= 0 {
		return covariance{}, false
	}

	mx, my := e.PmRA.Radians(), e.PmDec.Radians()

	c.xx += 2*dt*e.RAPmRA*sx*mx + dt*dt*mx*mx
	c.yy += 2*dt*e.DecPmDec*sy*my + dt*dt*my*my
	c.xy += dt*(e.RAPmDec*sx*my+e.DecPmRA*sy*mx) + dt*dt*e.PmRAPmDec*mx*my

	return c, true
}

// errorEpoch is the epoch t's errors hold at: their own, else t's, else
// J2000.0.
func errorEpoch(t resolve.Target) epoch.Time {
	switch {
	case !t.AstrometricError.Epoch.IsZero():
		return t.AstrometricError.Epoch
	case !t.Epoch.IsZero():
		return t.Epoch
	default:
		return epoch.J2000()
	}
}

// mahalanobis is Δᵀ C⁻¹ Δ for b's offset Δ from a on the plane tangent at a.
// It is false when b cannot be projected about a, or C is singular.
func mahalanobis(a, b coord.ICRS, c covariance) (float64, bool) {
	xi, eta, status := gofaext.Tpxes(b.RA().Radians(), b.Dec().Radians(), a.RA().Radians(), a.Dec().Radians())
	if status != 0 {
		return 0, false
	}

	det := c.xx*c.yy - c.xy*c.xy
	if !(det > 0) {
		return 0, false
	}

	return (c.yy*xi*xi - 2*c.xy*xi*eta + c.xx*eta*eta) / det, true
}
