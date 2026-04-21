// Package eph provides celestial body positions and velocities for astronomical computing.
//
// # Architecture
//
// The top-level [NewProvider] factory creates an [Ephemeris] for a given
// [Source] and kernel identifier, routing internally to specialised
// implementations:
//
//   - [Planets], [SmallBody], [Asteroids], [Comets] — JPL SPK/LSK kernels
//   - [Satellites] — NORAD TLE/GP element sets with SGP4 propagation
//
// This mirrors the catalog package's unified [catalog.Resolver] pattern:
// users rarely need to import subpackages directly.
//
// # Quick Start
//
//	// JPL planetary ephemeris (DE442)
//	p, err := eph.NewProvider(eph.Planets, "de442")
//	if err != nil { log.Fatal(err) }
//	defer p.Close()
//	state, _ := p.State(eph.Mars, t)
//
//	// Multi-kernel (deep historical + modern)
//	p, err := eph.NewProvider(eph.Planets, "de441_part-1", eph.WithKernel("de441_part-2"))
//
//	// NORAD satellite (ISS)
//	sat, err := eph.NewProvider(eph.Satellites, "ISS",
//	    eph.WithTLE(line1, line2))
//
// # Default Provider
//
// The [Default] provider uses algorithms from the IAU SOFA library (via gofaext):
//   - Sun: IAU 2000 Earth ephemeris (Epv00), providing ~0.1″ solar accuracy.
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
