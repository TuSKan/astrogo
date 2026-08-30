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
// plane** whose pole is tabulated per satellite and precesses, not to the
// J2000 ecliptic this package reads. Feeding such a set through unmodified
// propagates the right orbit in the wrong plane, and the error does not
// announce itself: the period is correct and the position is not.
//
// And two-body motion diverges quickly for the satellites most people want.
// The Galilean moons are locked in the Laplace resonance and Jupiter's J₂
// drives apsidal precession, so an unperturbed ellipse drifts measurably
// within weeks.
//
// What is here is therefore the correct machinery and the correct constants,
// not a satellite ephemeris. For real satellite positions use a kernel-backed
// provider ([github.com/TuSKan/astrogo/ephemeris.NewProvider] with
// [github.com/TuSKan/astrogo/ephemeris.Moons]).
package kepler
