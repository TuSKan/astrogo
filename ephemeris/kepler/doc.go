// Package kepler provides a lightweight, network-free alternative to
// SPK-kernel-backed ephemerides: propagating a position and velocity
// directly from classical heliocentric osculating orbital elements
// (semi-major axis, eccentricity, inclination, ascending node, argument
// of periapsis, mean anomaly) via analytic two-body Keplerian motion.
//
// [Provider] satisfies [github.com/TuSKan/astrogo/ephemeris/core.Provider]
// exactly like the SPK-kernel-backed providers in ephemeris/jpl, so a
// Kepler-propagated body drops straight into
// plan.NewAsteroid/NewComet/NewGenericBody with no new plan type needed.
//
// # Scope and accuracy
//
// This package implements pure elliptical (0 <= e < 1) two-body motion
// only. Hyperbolic/parabolic orbits ([ErrUnsupportedOrbit]) and an
// MPC-style constructor from perihelion distance and time of perihelion
// passage (rather than semi-major axis and mean anomaly) are deliberate,
// documented follow-ups, not built here.
//
// Because planetary perturbations are not modeled, a propagated
// position's accuracy drifts away from [Elements.Epoch]: typically
// arcseconds within days, arcminutes within months, for a main-belt
// asteroid's osculating elements. For higher-accuracy positions over
// longer spans, use a real SPK-kernel-backed provider instead — see
// [github.com/TuSKan/astrogo/ephemeris.NewProvider] with
// [github.com/TuSKan/astrogo/ephemeris.SmallBody].
//
// # Frame
//
// Elements are referred to the J2000 mean ecliptic and equinox, matching
// the convention MPC- and JPL-published orbital elements use.
// [Elements.StateAt] returns state in the ICRS-aligned mean equatorial
// frame, via a fixed rotation by the J2000 mean obliquity
// ([github.com/TuSKan/astrogo/constants.IAU2015.ObliquityJ2000]) —
// deliberately not an epoch-of-date obliquity, which would introduce a
// drift unrelated to the orbit's own real motion.
//
// # Central body
//
// The default central body is the Sun. [Elements.WithCentralBody] refers a
// set to a planet instead, which is what a satellite orbit needs, and
// [CentralBodyFor] supplies the parent's mass parameter without the caller
// having to choose between a planet's system and body values — a choice that
// is wrong by 12% at Pluto.
//
// # What a satellite orbit here is not
//
// Two things are missing before a planetary satellite computed this way
// should be believed, and both are real physics rather than plumbing.
//
// Published *mean* elements for a satellite are referred to a **Laplace
// plane** whose pole is tabulated per satellite, not to the J2000 ecliptic.
// [Elements.WithLaplacePlane] handles that; without it the angles are read
// against the wrong plane and, at Jupiter, put Io 16,500 km from where it
// belongs while the period stays right.
//
// Two secular corrections are available and must be used together:
// [Elements.WithPeriod] advances the mean anomaly at the published
// anomalistic period, and [Elements.WithSecularPrecession] turns the line of
// apsides back at its own rate. Applied as a pair they recover the sidereal
// period — for Io, 1.769137 days from the table's own two columns — and cut
// the six-month error against Horizons from 125,000 km to 74,000. Applied
// singly either one is worse than neither, by an order of magnitude.
//
// The tables also publish a node period, and it is deliberately not applied:
// doing so made the fit worse in both directions and no explanation for that
// was established.
//
// What remains unmodelled is the periodic perturbation. The Galilean moons are locked
// in the Laplace resonance and Jupiter's J₂ drives apsidal precession, so an
// unperturbed ellipse drifts — measured against Horizons over ten days from
// the elements' own epoch, by **up to 5,900 km** across the four Galilean
// satellites, with a median of 2,700 km. That is roughly one percent of Io's
// orbital radius and grows with the span.
//
// The drift is not spread evenly, which is worth knowing before trusting any
// one satellite. Comparing the two-body period against the published one:
// Io is off by 0.41% and Europa by 0.76%, while Ganymede and Callisto agree
// to about one part in 100,000. The inner two are the closest to Jupiter and
// the deepest in the resonance, and they carry almost all of the error.
//
// The tabulated node and apsis precession periods say what is missing:
// 30 years for Europa's node and 138 for Ganymede's, none of which this
// package applies.
//
// So what is here is the correct machinery, the correct constants and the
// correct frame — not a satellite ephemeris. Kilometre accuracy needs a
// kernel-backed provider ([github.com/TuSKan/astrogo/ephemeris.NewProvider]
// with [github.com/TuSKan/astrogo/ephemeris.Moons]); this is for a finder
// chart, not an occultation prediction.
package kepler
