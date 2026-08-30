package kepler

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/ephemeris/core"
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
// hyperbolic or parabolic Eccentricity is rejected by NewElements with
// ErrUnsupportedOrbit. Hyperbolic/parabolic propagation and an
// MPC-style FromPerihelion constructor (q, e, T_peri rather than a, M0)
// are deliberate, documented follow-ups, not built here.
//
// Elements is always constructed via NewElements, which validates
// immediately — there is no way to hold an Elements value whose fields
// haven't already passed Validate.
type Elements struct {
	epoch                                                 time.Time
	semiMajorAxis, eccentricity                           float64
	inclination, ascendingNode, argPeriapsis, meanAnomaly angle.Angle

	// central is the body the elements are referred to. The zero value
	// means the Sun, so an Elements built by NewElements and never given a
	// central body behaves exactly as it did before one existed.
	central CentralBody

	// plane is the reference plane the angles are measured in. Nil means
	// the J2000 ecliptic, which is what heliocentric elements use and what
	// this package read before satellites needed anything else.
	plane *LaplacePlane
}

// CentralBody is the body an element set orbits, and the mass parameter
// that governs that motion.
//
// It carries the identifier as well as the parameter because the two are
// needed at different stages and getting either wrong is silent: the mass
// parameter sets the orbital period, and the identifier is what the provider
// adds the satellite's position to in order to place it in the solar system.
// A satellite propagated with the right period about the wrong parent is a
// plausible orbit around nothing.
type CentralBody struct {
	// ID is the body the orbit is referred to.
	ID core.ID

	// GM is that body's mass parameter, in m³/s².
	//
	// For a satellite this is the parent's *body* parameter, not its system
	// parameter — see [CentralBodyFor], and constants.EphemerisSet for why
	// the difference reaches 12% at Pluto.
	GM float64
}

// sunCentre is the default central body: the Sun, with the IAU nominal mass
// parameter the heliocentric path has always used.
//
//nolint:gochecknoglobals // the default central body, read-only
var sunCentre = CentralBody{ID: core.Sun, GM: constants.IAU.SunGravitationalParameter.Value}

// LaplacePlane is the reference plane a satellite's published mean elements
// are referred to, named by the direction of its pole.
//
// It is not the ecliptic and not the planet's equator. It is the plane a
// satellite's orbit precesses about — for a close satellite, forced by the
// planet's oblateness towards the equator, and for a distant one by the Sun
// towards the planet's orbital plane. JPL tabulates its pole per satellite,
// alongside the elements, precisely because it differs from both and from
// one satellite to the next: at Jupiter it runs from (268.1°, 64.5°) for Io
// to (268.7°, 64.8°) for Callisto.
//
// Elements referred to such a plane and read as though they were ecliptic
// elements produce the right orbit in the wrong plane. At Jupiter the two are
// only about 2.2° apart — the planet's obliquity is 3.1°, so they cannot be
// far — and that is still enough to move Io by 16,500 km, four percent of its
// orbital radius and about 5 arcseconds seen from Earth, which is four times
// its own apparent diameter. Nothing about the resulting position looks
// wrong.
type LaplacePlane struct {
	// RA and Dec are the pole's right ascension and declination in the
	// ICRF equatorial frame, as JPL's satellite mean-element tables give
	// them.
	RA, Dec angle.Angle
}

// Pole returns the plane's pole as a unit vector in the ICRF equatorial
// frame.
func (lp LaplacePlane) Pole() vector.Vec3 {
	cd, sd := math.Cos(lp.Dec.Radians()), math.Sin(lp.Dec.Radians())
	ca, sa := math.Cos(lp.RA.Radians()), math.Sin(lp.RA.Radians())

	return vector.V3(cd*ca, cd*sa, sd)
}

// rotateToICRF turns a vector in the Laplace plane's own frame into the ICRF
// equatorial frame.
//
// The plane's frame has its z-axis along the pole and its x-axis along the
// plane's ascending node on the ICRF equator, which sits 90° ahead of the
// pole in right ascension. That makes the transformation the standard
// pole-to-frame pair: tilt by the pole's colatitude, then rotate the node
// into place.
func (lp LaplacePlane) rotateToICRF(v vector.Vec3) vector.Vec3 {
	colat := math.Pi/2 - lp.Dec.Radians()

	return v.RotateX(colat).RotateZ(lp.RA.Radians() + math.Pi/2)
}

// CentralBodyFor returns the central body for a satellite of the given
// planet, using that planet's own mass parameter from the current JPL
// ephemeris vintage.
//
// It exists so a caller does not have to choose between the system and body
// parameters, which is the error this pairing is most exposed to: the system
// value includes the satellites and is the wrong one for a satellite's own
// motion, by one part in 4,830 at Jupiter and by 12% at Pluto.
//
// Reports false for a body with no satellite system in the table — the Sun,
// the Moon, Mercury and Venus.
func CentralBodyFor(id core.ID) (CentralBody, bool) {
	gm, ok := map[core.ID]float64{
		core.Earth:   constants.Ephemeris.EarthGravitationalParameter.Value,
		core.Mars:    constants.Ephemeris.MarsGravitationalParameter.Value,
		core.Jupiter: constants.Ephemeris.JupiterGravitationalParameter.Value,
		core.Saturn:  constants.Ephemeris.SaturnGravitationalParameter.Value,
		core.Uranus:  constants.Ephemeris.UranusGravitationalParameter.Value,
		core.Neptune: constants.Ephemeris.NeptuneGravitationalParameter.Value,
		core.Pluto:   constants.Ephemeris.PlutoGravitationalParameter.Value,
	}[id]
	if !ok {
		return CentralBody{}, false
	}

	return CentralBody{ID: id, GM: gm}, true
}

// NewElements constructs a validated set of classical heliocentric
// osculating orbital elements, referred to the J2000 ecliptic frame —
// epoch is the osculation epoch meanAnomaly is given at; semiMajorAxis
// is in astronomical units; eccentricity must satisfy 0 <= e < 1;
// inclination/ascendingNode/argPeriapsis are the orbit's orientation
// angles in the J2000 ecliptic frame. Returns ErrInvalidElements/
// ErrUnsupportedOrbit immediately rather than deferring the failure to
// first use (e.g. inside StateAt) — a caller building Elements from
// external data (a resolve.Target's HasElements fields, a hand-typed
// literal) gets a construction-time error instead of a silently-broken
// propagator.
func NewElements(epoch time.Time, semiMajorAxis, eccentricity float64,
	inclination, ascendingNode, argPeriapsis, meanAnomaly angle.Angle,
) (Elements, error) {
	el := Elements{
		epoch: epoch, semiMajorAxis: semiMajorAxis, eccentricity: eccentricity,
		inclination: inclination, ascendingNode: ascendingNode,
		argPeriapsis: argPeriapsis, meanAnomaly: meanAnomaly,
	}
	if err := el.Validate(); err != nil {
		return Elements{}, err
	}

	return el, nil
}

// CentralBody reports the body these elements are referred to, defaulting to
// the Sun.
func (el Elements) CentralBody() CentralBody {
	if el.central.GM == 0 {
		return sunCentre
	}

	return el.central
}

// WithLaplacePlane returns a copy of el whose angles are read against the
// given plane rather than the J2000 ecliptic — the form JPL's satellite
// mean-element tables publish.
//
// This is what makes such a table usable. Without it the inclination, node
// and argument of periapsis are measured against the wrong plane — 2.2° away
// at Jupiter, which sounds small and puts Io 16,500 km from where it belongs,
// about 5 arcseconds seen from Earth against an apparent diameter of 1.2.
//
// It does not make the result an ephemeris. The elements are still propagated
// as unperturbed two-body motion, and the same tables publish the node and
// apsis precession periods that motion ignores — 137 years and 68 years for
// Ganymede. See this package's doc comment.
func (el Elements) WithLaplacePlane(plane LaplacePlane) Elements {
	el.plane = &plane

	return el
}

// LaplacePlane reports the plane el's angles are measured against, and
// whether one was set at all. Without one they are ecliptic elements.
func (el Elements) LaplacePlane() (LaplacePlane, bool) {
	if el.plane == nil {
		return LaplacePlane{}, false
	}

	return *el.plane, true
}

// WithCentralBody returns a copy of el referred to body instead of the Sun —
// the form published elements for a planetary satellite take.
//
// Only the central body changes. The elements themselves are still read as
// osculating elements in the J2000 ecliptic frame, which is *not* the frame
// most published satellite mean elements use: those are referred to a
// Laplace plane whose pole is tabulated per satellite and precesses. Feeding
// such a set through here unmodified propagates the right orbit in the wrong
// plane. See ephemeris/kepler's package doc.
func (el Elements) WithCentralBody(body CentralBody) Elements {
	el.central = body

	return el
}

// Epoch is the osculation epoch MeanAnomaly is given at.
func (el Elements) Epoch() time.Time { return el.epoch }

// SemiMajorAxis is in astronomical units.
func (el Elements) SemiMajorAxis() float64 { return el.semiMajorAxis }

// Eccentricity satisfies 0 <= e < 1.
func (el Elements) Eccentricity() float64 { return el.eccentricity }

// Inclination is the orbit's inclination in the J2000 ecliptic frame.
func (el Elements) Inclination() angle.Angle { return el.inclination }

// AscendingNode is the orbit's longitude of ascending node in the J2000
// ecliptic frame.
func (el Elements) AscendingNode() angle.Angle { return el.ascendingNode }

// ArgPeriapsis is the orbit's argument of periapsis in the J2000
// ecliptic frame.
func (el Elements) ArgPeriapsis() angle.Angle { return el.argPeriapsis }

// MeanAnomaly is the mean anomaly at Epoch.
func (el Elements) MeanAnomaly() angle.Angle { return el.meanAnomaly }

// Validate reports whether el's fields are finite and within the
// elliptical-orbit range this package supports. Elements built via
// NewElements has already passed this check; it is exported mainly for
// StateAt's own defense-in-depth and for anything within this package
// that constructs an Elements value via a bare struct literal (tests).
func (el Elements) Validate() error {
	if !finite(el.semiMajorAxis) || el.semiMajorAxis <= 0 {
		return fmt.Errorf("%w: semi-major axis %v must be finite and positive", ErrInvalidElements, el.semiMajorAxis)
	}

	if !finite(el.eccentricity) || el.eccentricity < 0 || el.eccentricity >= 1 {
		return fmt.Errorf("%w: eccentricity %v", ErrUnsupportedOrbit, el.eccentricity)
	}

	for name, a := range map[string]angle.Angle{
		"inclination": el.inclination, "ascending node": el.ascendingNode,
		"argument of periapsis": el.argPeriapsis, "mean anomaly": el.meanAnomaly,
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

	gm := el.CentralBody().GM                        // m^3/s^2
	auMeters := constants.IAU.AstronomicalUnit.Value // m
	aMeters := el.semiMajorAxis * auMeters

	nRadPerDay := math.Sqrt(gm/(aMeters*aMeters*aMeters)) * constants.Derived.JulianDaySeconds.Value

	dtDays := t.SubDays(el.epoch)
	m := el.meanAnomaly.Radians() + nRadPerDay*dtDays

	ea, err := SolveKepler(angle.Rad(m), el.eccentricity)
	if err != nil {
		return vector.Zero(), vector.Zero(), err
	}

	e := el.eccentricity
	cosE, sinE := ea.Cos(), ea.Sin()
	sqrtOneMinusE2 := math.Sqrt(1 - e*e)
	a := el.semiMajorAxis

	posPf := vector.V3(a*(cosE-e), a*sqrtOneMinusE2*sinE, 0)

	// Ėdot: rate of change of eccentric anomaly, rad/day — from
	// differentiating Kepler's equation E - e*sin(E) = M w.r.t. time.
	eDot := nRadPerDay / (1 - e*cosE)
	velPf := vector.V3(-a*sinE*eDot, a*sqrtOneMinusE2*cosE*eDot, 0)

	posRef := rotatePerifocalToEcliptic(posPf, el.inclination, el.ascendingNode, el.argPeriapsis)
	velRef := rotatePerifocalToEcliptic(velPf, el.inclination, el.ascendingNode, el.argPeriapsis)

	// Both paths end in the ICRF equatorial frame; they differ only in what
	// the orbit's angles were measured against.
	if lp := el.plane; lp != nil {
		return lp.rotateToICRF(posRef), lp.rotateToICRF(velRef), nil
	}

	return rotateEclipticToEquatorialJ2000(posRef), rotateEclipticToEquatorialJ2000(velRef), nil
}
