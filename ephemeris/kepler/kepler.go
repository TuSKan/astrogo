package kepler

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// finite reports whether v is neither NaN nor infinite.
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// Elements are classical heliocentric osculating orbital elements,
// referred to the J2000 mean ecliptic and equinox — the frame in which
// MPC- and JPL-published orbital elements are conventionally given.
//
// They come in the two forms elements are published in. [NewElements] takes
// a semi-major axis and a mean anomaly at an epoch, the asteroid form, and
// is elliptical only (0 <= e < 1). [FromPerihelion] takes a perihelion
// distance and time, the comet form, and accepts any e >= 0: ellipse,
// parabola or hyperbola.
//
// Elements is always constructed via one of the two, which validate
// immediately — there is no way to hold an Elements value whose fields
// haven't already passed Validate.
type Elements struct {
	epoch                                                 time.Time
	semiMajorAxis                                         unit.Length
	eccentricity                                          float64
	inclination, ascendingNode, argPeriapsis, meanAnomaly angle.Angle

	// perihelion is the perihelion distance of a set built by
	// FromPerihelion, which is propagated by universal variables from its
	// perihelion state at epoch; semiMajorAxis and meanAnomaly are then
	// unused. Zero means the mean-anomaly form NewElements builds.
	perihelion unit.Length

	// central is the body the elements are referred to. The zero value
	// means the Sun, so an Elements built by NewElements and never given a
	// central body behaves exactly as it did before one existed.
	central CentralBody

	// plane is the reference plane the angles are measured in. Nil means
	// the J2000 ecliptic, which is what heliocentric elements use and what
	// this package read before satellites needed anything else.
	plane *LaplacePlane

	// precession is the secular drift of the apsis. The zero value means
	// none, which is what a fixed two-body ellipse has.
	precession SecularPrecession

	// periodDays overrides the mean motion implied by the semi-major axis
	// and the central body's mass parameter. Zero means derive it.
	periodDays float64
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

	// GM is the mass parameter governing the orbit, in m³/s².
	//
	// For a satellite that is **μ = G(M_primary + M_satellite)**, not the
	// primary's mass alone: two-body relative motion is governed by the sum.
	// The satellite is usually negligible and the primary's own parameter is
	// then the right approximation, which is what [CentralBodyFor] returns —
	// but Charon is 12% of Pluto, and there the sum is what matters. See
	// [CentralBodyFor] for how far each approximation is off.
	GM float64
}

// sunCenter is the default central body: the Sun, with the IAU nominal mass
// parameter the heliocentric path has always used.
var sunCenter = CentralBody{ID: core.Sun, GM: constants.IAU.SunGravitationalParameter.Value}

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

// SecularPrecession is the steady turning of an orbit's line of apsides.
//
// A two-body ellipse is fixed in space; a real satellite's is not. JPL's
// satellite tables publish the period of the turn beside the elements, and
// applying it is what keeps a propagation from drifting in orientation.
//
// # It only works with the published period
//
// The table's "P (days)" is the **anomalistic** period — periapsis to
// periapsis — not the sidereal one. For Io the sidereal period is 1.769138
// days and the table gives 1.762732, and the difference is exactly the apsis
// rate: n_sidereal + dω/dt = 204.2283°/day, which is 1.762733 days. Verified
// to seven digits.
//
// So mean anomaly must advance at that anomalistic rate, which means
// [Elements.WithPeriod] as well as this. Applying one without the other makes
// the result worse than plain two-body motion — measured, thirteen times
// worse — because the two corrections are two halves of the same statement.
//
// # What is deliberately absent
//
// The tables also publish a node period, and it is not applied. Doing so made
// the fit against Horizons worse in both directions — 74,038 km becomes
// 97,412 or 134,570 — and no explanation for that was established. An
// unexplained term that measurably hurts is not shipped on the grounds that
// the table contains it.
type SecularPrecession struct {
	// ApsisPeriod is the time for the line of apsides to complete a turn,
	// in Julian years — the "P apsis" column. Zero means none.
	ApsisPeriod float64
}

// rate returns the drift of the argument of periapsis, in radians per day.
//
// Negative for a positive published period. That is not a sign error: mean
// anomaly is being advanced at the anomalistic rate, which already carries
// the apsis motion, so the orientation has to be walked back by the same
// amount to leave the sidereal rate behind. Measured both ways — the other
// sign is eighteen times worse.
func (p SecularPrecession) rate() float64 {
	const daysPerJulianYear = 365.25

	if p.ApsisPeriod == 0 {
		return 0
	}

	return -2 * math.Pi / (p.ApsisPeriod * daysPerJulianYear)
}

// CentralBodyFor returns the central body for a satellite of the given
// planet, using that planet's own mass parameter from the current JPL
// ephemeris vintage.
//
// # Which mass parameter is right
//
// Two-body relative motion is governed by **μ = G(M_primary + M_satellite)**,
// so strictly neither published value is it. The two available are the
// planet's own parameter and the system parameter, which is the planet plus
// *all* its satellites:
//
//   - For a negligible satellite the planet's own parameter is the better
//     approximation. Io is 4.7e-05 of Jupiter, while Jupiter's system
//     parameter overshoots by 2.1e-04 because it also carries Europa,
//     Ganymede and Callisto. This function returns the planet's own.
//   - For a satellite that dominates its system the sum is what matters, and
//     the system parameter *is* that sum. Charon is 12% of Pluto: its
//     published period of 6.3872 days comes out at 6.3871 with Pluto's
//     system parameter and 6.7648 — six percent long — with Pluto's own.
//
// So for Charon, and anything else massive relative to its primary, set
// [CentralBody.GM] to constants.Ephemeris.PlutoSystemGravitationalParameter
// rather than taking the default here.
//
// An earlier version of this comment had that backwards, and said the system
// parameter was the wrong one at Pluto. The period measurement settled it.
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
// epoch is the osculation epoch meanAnomaly is given at; eccentricity must
// satisfy 0 <= e < 1;
// inclination/ascendingNode/argPeriapsis are the orbit's orientation
// angles in the J2000 ecliptic frame. Returns ErrInvalidElements/
// ErrUnsupportedOrbit immediately rather than deferring the failure to
// first use (e.g. inside StateAt) — a caller building Elements from
// external data (a resolve.Target's HasElements fields, a hand-typed
// literal) gets a construction-time error instead of a silently-broken
// propagator.
// Orbital elements are published with the semi-major axis in astronomical
// units, so it is almost always [unit.AU](a) here.
func NewElements(epoch time.Time, semiMajorAxis unit.Length, eccentricity float64,
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

// FromPerihelion constructs a validated set of heliocentric osculating
// elements from a perihelion time, perihelion distance and eccentricity —
// the form the Minor Planet Center publishes comets in — for any
// eccentricity e >= 0: an ellipse, a parabola or a hyperbola. The angles are
// referred to the J2000 ecliptic, as for [NewElements].
//
// It never forms the semi-major axis q/(1−e). That is infinite for a
// parabola and negative for a hyperbola, and for the near-parabolic comets
// that make up most of the MPC's list — C/2023 A3 has e = 1.000178 — it is
// a large number built from a subtraction that has already lost four of its
// sixteen digits. [Elements.StateAt] instead propagates the set by
// universal variables from its perihelion state, one path for all three
// conics that stays well conditioned as e crosses 1.
//
// [Elements.Epoch] reports perihelionTime and [Elements.MeanAnomaly] zero,
// since that is where the propagation starts. A published comet orbit
// osculates at an epoch of its own, which is usually not the perihelion
// time, and it is near that epoch that two-body motion is closest to the
// real orbit; see this package's doc comment.
//
// Orbital elements are published with the perihelion distance in
// astronomical units, so it is almost always [unit.AU](q) here.
func FromPerihelion(perihelionTime time.Time, perihelionDistance unit.Length, eccentricity float64,
	inclination, ascendingNode, argPeriapsis angle.Angle,
) (Elements, error) {
	if !finite(perihelionDistance.AU()) || perihelionDistance <= 0 {
		return Elements{}, fmt.Errorf("%w: perihelion distance %v must be finite and positive",
			ErrInvalidElements, perihelionDistance)
	}

	el := Elements{
		epoch: perihelionTime, perihelion: perihelionDistance, eccentricity: eccentricity,
		inclination: inclination, ascendingNode: ascendingNode, argPeriapsis: argPeriapsis,
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
		return sunCenter
	}

	return el.central
}

// WithSecularPrecession returns a copy of el whose apsis and node drift at
// the given rates rather than staying fixed.
//
// JPL's satellite tables publish both periods beside the elements. Applying
// them removes the largest part of the two-body error for a close satellite:
// Io's apsis turns 0.74° per day, so over ten days a fixed ellipse is 7.4°
// out in the orientation of its own long axis.
func (el Elements) WithSecularPrecession(p SecularPrecession) Elements {
	el.precession = p

	return el
}

// SecularPrecession reports the drift applied to the apsis and node.
func (el Elements) SecularPrecession() SecularPrecession { return el.precession }

// WithPeriod returns a copy of el whose mean anomaly advances at the given
// period rather than the one implied by the semi-major axis and the central
// body's mass parameter.
//
// Published satellite elements come with their own period, and it is not the
// two-body one: the table's value is anomalistic, and for Io it differs from
// the two-body figure by 0.4%, ten minutes per revolution. Use it together
// with [Elements.WithSecularPrecession] — separately, either makes the result
// worse than using neither.
//
// Zero restores the derived mean motion.
func (el Elements) WithPeriod(days float64) Elements {
	el.periodDays = days

	return el
}

// Period reports the period the mean anomaly advances at, in days: the one
// supplied by [Elements.WithPeriod], or zero when it is derived.
func (el Elements) Period() float64 { return el.periodDays }

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

// Epoch is the osculation epoch MeanAnomaly is given at; for a set built by
// [FromPerihelion], the perihelion time.
func (el Elements) Epoch() time.Time { return el.epoch }

// SemiMajorAxis is the orbit's semi-major axis: positive for an ellipse,
// negative for a hyperbola by the usual convention, and +Inf for a parabola.
func (el Elements) SemiMajorAxis() unit.Length {
	if el.perihelion == 0 {
		return el.semiMajorAxis
	}

	if el.eccentricity == 1 {
		return unit.Length(math.Inf(1))
	}

	return el.perihelion / unit.Length(1-el.eccentricity)
}

// PerihelionDistance is the orbit's closest approach to its central body,
// a(1−e) — defined for every conic, including the parabola, whose
// semi-major axis is not.
func (el Elements) PerihelionDistance() unit.Length {
	if el.perihelion != 0 {
		return el.perihelion
	}

	return el.semiMajorAxis * unit.Length(1-el.eccentricity)
}

// Eccentricity satisfies 0 <= e < 1 for a set built by [NewElements], and
// e >= 0 for one built by [FromPerihelion].
func (el Elements) Eccentricity() float64 { return el.eccentricity }

// Inclination is the orbit's inclination in the J2000 ecliptic frame.
func (el Elements) Inclination() angle.Angle { return el.inclination }

// AscendingNode is the orbit's longitude of ascending node in the J2000
// ecliptic frame.
func (el Elements) AscendingNode() angle.Angle { return el.ascendingNode }

// ArgPeriapsis is the orbit's argument of periapsis in the J2000
// ecliptic frame.
func (el Elements) ArgPeriapsis() angle.Angle { return el.argPeriapsis }

// MeanAnomaly is the mean anomaly at Epoch; zero for a set built by
// [FromPerihelion], whose epoch is the perihelion time.
func (el Elements) MeanAnomaly() angle.Angle { return el.meanAnomaly }

// Validate reports whether el's fields are finite and within the range its
// form supports. Elements built via NewElements or FromPerihelion has already
// passed this check; it is exported mainly for StateAt's own
// defense-in-depth and for anything within this package that constructs an
// Elements value via a bare struct literal (tests).
//
// The eccentricity is checked before anything else, so an orbit the form
// cannot represent is [ErrUnsupportedOrbit] whatever else is wrong with it. A
// comet converted for NewElements through a = q/(1−e) has a negative or
// infinite semi-major axis precisely because e >= 1, and the caller needs to
// hear that, not that the data is bad.
func (el Elements) Validate() error {
	if el.perihelion != 0 {
		return el.validatePerihelionForm()
	}

	if !finite(el.eccentricity) || el.eccentricity < 0 || el.eccentricity >= 1 {
		return fmt.Errorf("%w: eccentricity %v; NewElements takes 0 <= e < 1, FromPerihelion any e >= 0",
			ErrUnsupportedOrbit, el.eccentricity)
	}

	if !finite(el.semiMajorAxis.AU()) || el.semiMajorAxis <= 0 {
		return fmt.Errorf("%w: semi-major axis %v must be finite and positive",
			ErrInvalidElements, el.semiMajorAxis)
	}

	return el.validateAngles()
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
// shared vernal-equinox (X) axis by the FIXED obliquity that defines the
// J2000 ecliptic of published elements (elementsEclipticObliquity) —
// deliberately not gofaext.Obl06(t)'s epoch-of-date obliquity. Elements are referred to the J2000 ecliptic
// by definition, so mixing in an epoch-of-date obliquity here would
// introduce a spurious drift unrelated to the orbit's own real motion.
func rotateEclipticToEquatorialJ2000(v vector.Vec3) vector.Vec3 {
	return v.RotateX(elementsEclipticObliquity)
}

// elementsEclipticObliquity is the obliquity that defines the J2000 ecliptic
// published osculating elements are referred to: 84381.448″, the IAU 1976
// value. Every Horizons ELEMENTS table is headed "IAU76/J2000 helio. ecliptic
// osc. elements", and SBDB's elements are the same solution.
//
// It is not constants.IAU.ObliquityJ2000, the IAU 2006 value of 84381.406″,
// which is right for a mean obliquity in a precession model and wrong here:
// the 0.042″ between them is a rotation about the equinox, and rotating JPL's
// elements by the IAU 2006 value tilted every position this package produced
// from them — 33 km for C/2023 A3 at 2.5 AU, and the 0.04″ at dt = 0 that the
// 433 Eros validation had put down to perturbations (#391).
//
// Standish's approximate planetary elements, of which PlutoElements is a row,
// convert with 23.43928° (84381.408″). Rotating them by this value instead
// moves Pluto by 0.040″, against the 0.14 AU its own two-body fit is off by.
const elementsEclipticObliquity = 84381.448 * math.Pi / 648000

// StateAt returns el's heliocentric position (AU) and velocity (AU/day)
// at time t, in the ICRS-aligned mean equatorial frame, via two-body
// Keplerian propagation.
//
// A set built by [NewElements] is propagated through Kepler's equation in
// the eccentric anomaly; one built by [FromPerihelion], of any eccentricity,
// by universal variables from its perihelion state.
//
// This is two-body motion only: planetary perturbations are not modeled,
// so accuracy drifts away from Epoch — arcseconds within days, arcminutes
// within months, for a typical main-belt asteroid. For higher-accuracy
// positions over longer spans, use a real SPK-kernel-backed provider
// instead (ephemeris.NewProvider with eph.SmallBody).
func (el Elements) StateAt(t time.Time) (pos, vel vector.Vec3, err error) {
	if err := el.Validate(); err != nil {
		return vector.Zero(), vector.Zero(), err
	}

	dtDays := t.Sub(el.epoch).Days()

	var posPf, velPf vector.Vec3
	if el.perihelion != 0 {
		posPf, velPf, err = el.perifocalUniversal(dtDays)
	} else {
		posPf, velPf, err = el.perifocalClassical(dtDays)
	}

	if err != nil {
		return vector.Zero(), vector.Zero(), err
	}

	pos, vel = el.orient(posPf, velPf, dtDays)

	return pos, vel, nil
}

// validatePerihelionForm is Validate for a set built by FromPerihelion.
func (el Elements) validatePerihelionForm() error {
	if !finite(el.eccentricity) || el.eccentricity < 0 {
		return fmt.Errorf("%w: eccentricity %v; FromPerihelion takes any e >= 0",
			ErrUnsupportedOrbit, el.eccentricity)
	}

	if !finite(el.perihelion.AU()) || el.perihelion <= 0 {
		return fmt.Errorf("%w: perihelion distance %v must be finite and positive",
			ErrInvalidElements, el.perihelion)
	}

	if el.eccentricity >= 1 && el.periodDays != 0 {
		return fmt.Errorf("%w: a period of %v days for an orbit with e = %v, which does not close",
			ErrInvalidElements, el.periodDays, el.eccentricity)
	}

	return el.validateAngles()
}

// validateAngles checks the four angles are finite.
func (el Elements) validateAngles() error {
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

// perifocalClassical is the state dtDays after epoch in the perifocal frame
// (AU, AU/day), from Kepler's equation in the eccentric anomaly.
func (el Elements) perifocalClassical(dtDays float64) (posPf, velPf vector.Vec3, err error) {
	gm := el.CentralBody().GM // m^3/s^2
	aMeters := el.semiMajorAxis.Meters()

	nRadPerDay := math.Sqrt(gm/(aMeters*aMeters*aMeters)) * constants.Derived.JulianDaySeconds.Value
	if el.periodDays != 0 {
		nRadPerDay = 2 * math.Pi / el.periodDays
	}

	m := el.meanAnomaly.Radians() + nRadPerDay*dtDays

	ea, err := SolveKepler(angle.Rad(m), el.eccentricity)
	if err != nil {
		return vector.Zero(), vector.Zero(), err
	}

	e := el.eccentricity
	cosE, sinE := ea.Cos(), ea.Sin()
	sqrtOneMinusE2 := math.Sqrt(1 - e*e)
	a := el.semiMajorAxis.AU()

	posPf = vector.V3(a*(cosE-e), a*sqrtOneMinusE2*sinE, 0)

	// Ėdot: rate of change of eccentric anomaly, rad/day — from
	// differentiating Kepler's equation E - e*sin(E) = M w.r.t. time.
	eDot := nRadPerDay / (1 - e*cosE)
	velPf = vector.V3(-a*sinE*eDot, a*sqrtOneMinusE2*cosE*eDot, 0)

	return posPf, velPf, nil
}

// perifocalUniversal is the state dtDays after the perihelion time in the
// perifocal frame (AU, AU/day), by universal variables: Vallado,
// "Fundamentals of Astrodynamics and Applications", Algorithm 8, and Curtis,
// "Orbital Mechanics for Engineering Students", Algorithms 3.3 and 3.4,
// specialized to a start at perihelion, where the radial velocity is zero.
//
// With α = 1/a = (1−e)/q, the universal anomaly χ solves
//
//	F(χ) = e·χ³·S(αχ²) + q·χ − √μ·Δt = 0,
//
// and the Lagrange coefficients built on it carry the perihelion state
// forward. Nothing here divides by 1−e, which is what lets one path serve
// the ellipse, the parabola and the hyperbola alike.
func (el Elements) perifocalUniversal(dtDays float64) (posPf, velPf vector.Vec3, err error) {
	q := el.perihelion.AU()
	e := el.eccentricity
	alpha := (1 - e) / q

	auMeters := unit.AU(1).Meters()
	day := constants.Derived.JulianDaySeconds.Value
	mu := el.CentralBody().GM * day * day / (auMeters * auMeters * auMeters) // AU³/day²

	if el.periodDays != 0 {
		// Validate admits a period only for a closed orbit. Holding it means
		// the mass parameter that gives an orbit of this size that period.
		n := 2 * math.Pi / el.periodDays
		mu = n * n / (alpha * alpha * alpha)
	}

	// A closed orbit repeats, so fold the interval to within half a period of
	// perihelion. The universal anomaly then stays within ±π·√a, where the
	// Stumpff functions are best conditioned, however many revolutions away
	// t is.
	dt := dtDays

	if alpha > 0 {
		period := 2 * math.Pi / math.Sqrt(mu*alpha*alpha*alpha)
		dt -= period * math.Round(dt/period)
	}

	sqrtMu := math.Sqrt(mu)

	chi, err := solveUniversal(q, e, alpha, sqrtMu, dt)
	if err != nil {
		return vector.Zero(), vector.Zero(), err
	}

	c, s := stumpff(alpha * chi * chi)
	r := q + e*chi*chi*c
	v0 := math.Sqrt(mu * (1 + e) / q) // speed at perihelion, from vis-viva

	f := 1 - chi*chi*c/q
	g := dt - chi*chi*chi*s/sqrtMu
	fDot := sqrtMu / (r * q) * (alpha*chi*chi*chi*s - chi)
	gDot := 1 - chi*chi*c/r

	// The perihelion state is (q, 0, 0) with velocity (0, v0, 0).
	return vector.V3(f*q, g*v0, 0), vector.V3(fDot*q, gDot*v0, 0), nil
}

// solveUniversal solves the universal Kepler equation of perifocalUniversal
// for χ, by Newton's method held inside a bracket.
//
// The bracket is what makes it safe for every conic. F'(χ) is the distance
// r, which is never less than q, so F(χ) >= q·χ − √μ·Δt and the root for
// Δt > 0 lies in [0, √μ·Δt/q]. A Newton step that leaves the bracket, or
// is not a number — sinh overflows far out on a strongly hyperbolic orbit —
// becomes a bisection instead.
//
// The iteration starts from a lower bound on the root: √a·M for an ellipse,
// since E >= M, and √(−a)·asinh(M/e) for a hyperbola, since
// e·sinh H = M + H >= M. The hyperbolic one is what makes a hyperbola far
// from perihelion converge in a few steps. F grows there like sinh, and
// Newton's method from above an exponential descends by about one unit of
// anomaly per step: started from the bracket's upper end instead, the
// iteration runs out of steps a thousand years out on every hyperbola this
// package tests, 1I/'Oumuamua and C/2023 A3 among them. The parabola starts
// from the upper end, where F is a convex cubic. The bracket's lower end stays at zero rather
// than at the starting bound, so a bound that rounding put a hair above the
// root cannot close it.
//
// F is odd in χ, so Δt < 0 is solved as −Δt and the sign restored.
func solveUniversal(q, e, alpha, sqrtMu, dt float64) (float64, error) {
	const (
		tolerance = 1e-13
		maxIter   = 200
	)

	target := sqrtMu * math.Abs(dt)
	if target == 0 {
		return 0, nil
	}

	lo, hi := 0.0, target/q
	chi := hi

	switch {
	case alpha > 0:
		chi = target * alpha
	case alpha < 0:
		sqrtNegA := math.Sqrt(-1 / alpha)
		chi = sqrtNegA * math.Asinh(target*(-alpha)/sqrtNegA/e)
	}

	if !(chi > lo && chi <= hi) {
		chi = hi
	}

	for range maxIter {
		c, s := stumpff(alpha * chi * chi)
		f := e*chi*chi*chi*s + q*chi - target
		fp := e*chi*chi*c + q

		if f < 0 {
			lo = chi
		} else {
			hi = chi
		}

		// A converged step is taken whatever the bracket says. At the root
		// the iterate has just become one end of the bracket, and a step of
		// a few ulps cannot land strictly inside it; bisecting then would
		// throw the converged value away for a midpoint.
		step := f / fp
		next := chi - step

		if math.Abs(step) <= tolerance*math.Max(1, math.Abs(chi)) && finite(next) {
			return math.Copysign(next, dt), nil
		}

		if !(next > lo && next < hi) {
			next = lo + (hi-lo)/2
		}

		chi = next
	}

	return 0, fmt.Errorf("%w: universal anomaly after %d iterations (q=%v, e=%v, Δt=%v d)",
		ErrKeplerNoConverge, maxIter, q, e, dt)
}

// stumpff returns the Stumpff functions C(z) and S(z).
//
// The closed forms cancel for small |z|, with a relative error near 6ε/|z|,
// and small |z| is exactly where a near-parabolic comet spends a year either
// side of perihelion: |z| is about 1e-3 for C/2022 E3. Below |z| = 0.1 the
// Maclaurin series is used instead, which reaches full double precision well
// within twenty terms:
//
//	C(z) = Σ (−z)^k / (2k+2)!,   S(z) = Σ (−z)^k / (2k+3)!
func stumpff(z float64) (c, s float64) {
	if math.Abs(z) < 0.1 {
		tc, ts := 0.5, 1.0/6
		c, s = tc, ts

		for k := 1; k < 20; k++ {
			tc *= -z / float64((2*k+1)*(2*k+2))
			ts *= -z / float64((2*k+2)*(2*k+3))
			c += tc
			s += ts
		}

		return c, s
	}

	if z > 0 {
		sz := math.Sqrt(z)

		return (1 - math.Cos(sz)) / z, (sz - math.Sin(sz)) / (sz * sz * sz)
	}

	sz := math.Sqrt(-z)

	return (math.Cosh(sz) - 1) / -z, (math.Sinh(sz) - sz) / (sz * sz * sz)
}

// orient carries a perifocal state into the ICRS-aligned equatorial frame,
// advancing the apsis by any secular precession over dtDays first.
func (el Elements) orient(posPf, velPf vector.Vec3, dtDays float64) (pos, vel vector.Vec3) {
	// The apsis and node are advanced to the requested epoch before the
	// orientation is applied. Only the orbit's orientation drifts; its size
	// and shape are unchanged, which is what makes a secular rate a rate
	// rather than a re-fit.
	argp := angle.Rad(el.argPeriapsis.Radians() + el.precession.rate()*dtDays)

	posRef := rotatePerifocalToEcliptic(posPf, el.inclination, el.ascendingNode, argp)
	velRef := rotatePerifocalToEcliptic(velPf, el.inclination, el.ascendingNode, argp)

	// Both paths end in the ICRF equatorial frame; they differ only in what
	// the orbit's angles were measured against.
	if lp := el.plane; lp != nil {
		return lp.rotateToICRF(posRef), lp.rotateToICRF(velRef)
	}

	return rotateEclipticToEquatorialJ2000(posRef), rotateEclipticToEquatorialJ2000(velRef)
}
