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
package kepler
