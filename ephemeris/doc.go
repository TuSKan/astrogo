// Package ephemeris provides celestial body positions and velocities for astronomical computing.
//
// # Design
//
// The ephemeris package abstracts the source of body states (position and velocity)
// through the [Provider] interface. This allows for simple analytical series (SOFA),
// high-precision JPL binary ephemeris (SPICE/DE4xx), or custom providers.
//
// # Implementation
//
// The [Default] provider uses algorithms from the IAU SOFA library (via [gofaext]):
//   - Sun: IAU 2000 Earth ephemeris (Epv00), providing ~0.1" solar accuracy.
//   - Moon: Meeus 1998 algorithm (Moon98), providing geocentric GCRS-like state.
//
// # Coordinates and Frames
//
// Providers return [State] vectors (position and velocity) in astronomical units
// (AU) and AU/day. These are geocentric inertial states, typically in an
// ICRS-compatible frame.
//
// Use the [ToICRS] helper to convert Cartesian geocentric positions into
// spherical [coord.ICRS] coordinates.
package ephemeris
