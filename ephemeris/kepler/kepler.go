package kepler

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// finite reports whether v is neither NaN nor infinite.
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// Elements are classical heliocentric osculating orbital elements,
// referred to the J2000 mean ecliptic and equinox — the frame in which
// MPC- and JPL-published orbital elements are conventionally given.
//
// Only elliptical orbits (0 <= Eccentricity < 1) are supported; a
// hyperbolic or parabolic Eccentricity is rejected by Validate/StateAt
// with ErrUnsupportedOrbit. Hyperbolic/parabolic propagation and an
// MPC-style FromPerihelion constructor (q, e, T_peri rather than a, M0)
// are deliberate, documented follow-ups, not built here.
type Elements struct {
	// Epoch is the osculation epoch MeanAnomaly is given at.
	Epoch time.Time
	// SemiMajorAxis is in astronomical units.
	SemiMajorAxis float64
	// Eccentricity must satisfy 0 <= e < 1.
	Eccentricity float64
	// Inclination, AscendingNode, and ArgPeriapsis are the orbit's
	// orientation angles in the J2000 ecliptic frame.
	Inclination   angle.Angle
	AscendingNode angle.Angle
	ArgPeriapsis  angle.Angle
	// MeanAnomaly is the mean anomaly at Epoch.
	MeanAnomaly angle.Angle
}

// Validate reports whether el's fields are finite and within the
// elliptical-orbit range this package supports.
func (el Elements) Validate() error {
	if !finite(el.SemiMajorAxis) || el.SemiMajorAxis <= 0 {
		return fmt.Errorf("%w: semi-major axis %v must be finite and positive", ErrInvalidElements, el.SemiMajorAxis)
	}

	if !finite(el.Eccentricity) || el.Eccentricity < 0 || el.Eccentricity >= 1 {
		return fmt.Errorf("%w: eccentricity %v", ErrUnsupportedOrbit, el.Eccentricity)
	}

	for name, a := range map[string]angle.Angle{
		"inclination": el.Inclination, "ascending node": el.AscendingNode,
		"argument of periapsis": el.ArgPeriapsis, "mean anomaly": el.MeanAnomaly,
	} {
		if !finite(a.Radians()) {
			return fmt.Errorf("%w: %s is not finite", ErrInvalidElements, name)
		}
	}

	return nil
}

// SolveKepler solves Kepler's equation E - e*sin(E) = M for the
// eccentric anomaly E, given the mean anomaly M and eccentricity e
// (0 <= e < 1), via Newton-Raphson iteration starting from the
// Eccentricity-dependent initial guess E0 = M + e*sin(M) (or pi for
// e>0.9, where that guess converges poorly). Converges to 1e-12 rad or
// fails with ErrKeplerNoConverge after 50 iterations; the result is
// checked finite before being reported as a success, so a degenerate
// iteration can never silently return a NaN/Inf eccentric anomaly.
func SolveKepler(meanAnomaly angle.Angle, ecc float64) (angle.Angle, error) {
	const (
		tolerance = 1e-12
		maxIter   = 50
	)

	if !finite(ecc) || ecc < 0 || ecc >= 1 {
		return angle.Zero(), fmt.Errorf("%w: eccentricity %v", ErrUnsupportedOrbit, ecc)
	}

	m := meanAnomaly.Wrap2Pi().Radians()
	if !finite(m) {
		return angle.Zero(), fmt.Errorf("%w: mean anomaly is not finite", ErrKeplerNoConverge)
	}

	e := m + ecc*math.Sin(m)
	if ecc > 0.9 {
		e = math.Pi
	}

	converged := false

	for range maxIter {
		f := e - ecc*math.Sin(e) - m
		fp := 1 - ecc*math.Cos(e)
		de := f / fp
		e -= de

		if math.Abs(de) < tolerance {
			converged = true
			break
		}
	}

	if !converged || !finite(e) {
		return angle.Zero(), fmt.Errorf("%w: after %d iterations", ErrKeplerNoConverge, maxIter)
	}

	return angle.Rad(e), nil
}

// rotatePerifocalToEcliptic transforms a perifocal-frame vector into the
// J2000 mean ecliptic frame via the classical R_z(Omega)*R_x(i)*R_z(omega)
// orbital-orientation rotation (Vallado, "Fundamentals of Astrodynamics
// and Applications" — independently verified against the perifocal
// coordinate system's published transformation matrix, since a sign
// error here is the single most common defect in any two-body
// propagator). Composed as three successive rotations of the vector
// itself (this package's vector.Vec3.RotateZ/RotateX rotate the vector,
// not the frame), applied argument-of-periapsis first, so this method
// chain applies innermost-first exactly like the matrix product does:
// v.RotateZ(argp) is R_z(omega)*v, then RotateX(incl) is R_x(i) of that,
// then RotateZ(node) is R_z(Omega) of that, giving
// R_z(Omega)*R_x(i)*R_z(omega)*v overall.
func rotatePerifocalToEcliptic(v vector.Vec3, incl, node, argp angle.Angle) vector.Vec3 {
	return v.RotateZ(argp.Radians()).RotateX(incl.Radians()).RotateZ(node.Radians())
}

// rotateEclipticToEquatorialJ2000 transforms a J2000 mean ecliptic vector
// into the ICRS-aligned mean equatorial frame by rotating about the
// shared vernal-equinox (X) axis by the FIXED J2000 mean obliquity
// (constants.IAU.ObliquityJ2000) — deliberately not gofaext.Obl06(t)'s
// epoch-of-date obliquity. Elements are referred to the J2000 ecliptic
// by definition, so mixing in an epoch-of-date obliquity here would
// introduce a spurious drift unrelated to the orbit's own real motion.
func rotateEclipticToEquatorialJ2000(v vector.Vec3) vector.Vec3 {
	return v.RotateX(constants.IAU.ObliquityJ2000.Value)
}

// StateAt returns el's heliocentric position (AU) and velocity (AU/day)
// at time t, in the ICRS-aligned mean equatorial frame, via two-body
// Keplerian propagation.
//
// This is elliptical two-body motion only: planetary perturbations are
// not modeled, so accuracy drifts away from Epoch — arcseconds within
// days, arcminutes within months, for a typical main-belt asteroid. For
// higher-accuracy positions over longer spans, use a real
// SPK-kernel-backed provider instead (ephemeris.NewProvider with
// eph.SmallBody).
func (el Elements) StateAt(t time.Time) (pos, vel vector.Vec3, err error) {
	if err := el.Validate(); err != nil {
		return vector.Zero(), vector.Zero(), err
	}

	gm := constants.IAU.SunGravitationalParameter.Value // m^3/s^2
	auMeters := constants.IAU.AstronomicalUnit.Value    // m
	aMeters := el.SemiMajorAxis * auMeters

	nRadPerDay := math.Sqrt(gm/(aMeters*aMeters*aMeters)) * constants.Derived.JulianDaySeconds.Value

	dtDays := t.SubDays(el.Epoch)
	m := el.MeanAnomaly.Radians() + nRadPerDay*dtDays

	ea, err := SolveKepler(angle.Rad(m), el.Eccentricity)
	if err != nil {
		return vector.Zero(), vector.Zero(), err
	}

	e := el.Eccentricity
	cosE, sinE := ea.Cos(), ea.Sin()
	sqrtOneMinusE2 := math.Sqrt(1 - e*e)
	a := el.SemiMajorAxis

	posPf := vector.V3(a*(cosE-e), a*sqrtOneMinusE2*sinE, 0)

	// Ėdot: rate of change of eccentric anomaly, rad/day — from
	// differentiating Kepler's equation E - e*sin(E) = M w.r.t. time.
	eDot := nRadPerDay / (1 - e*cosE)
	velPf := vector.V3(-a*sinE*eDot, a*sqrtOneMinusE2*cosE*eDot, 0)

	posEcl := rotatePerifocalToEcliptic(posPf, el.Inclination, el.AscendingNode, el.ArgPeriapsis)
	velEcl := rotatePerifocalToEcliptic(velPf, el.Inclination, el.AscendingNode, el.ArgPeriapsis)

	return rotateEclipticToEquatorialJ2000(posEcl), rotateEclipticToEquatorialJ2000(velEcl), nil
}
