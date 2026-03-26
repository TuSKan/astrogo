// Package ephemeris provides celestial body positions for astronomical computing.
//
// # Design
//
// The ephemeris package abstracts the source of body positions through the
// [Provider] interface. This allows for simple analytical series (v1),
// high-precision JPL binary ephemeris (DE4xx), or custom providers.
//
// # Implementation
//
// The [Default] provider uses algorithms from the IAU SOFA library (via [gofa]):
//   - Sun: IAU 2000 Earth ephemeris (Epv00), providing ~0.1" solar accuracy.
//   - Moon: Meeus 1998 algorithm (Moon98), providing ~0.5" lunar accuracy.
//
// # Coordinates
//
// The high-level [Position] helper returns coordinates in the [coord.ICRS]
// frame. In v1, these are geocentric apparent positions (the site argument 
// is reserved for future topocentric parallax implementation).
package ephemeris
