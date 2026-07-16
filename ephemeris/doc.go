// Package ephemeris provides celestial body positions and velocities for astronomical computing.
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
//	p, err := eph.NewProvider(ctx, eph.Planets, "de442")
//	if err != nil { log.Fatal(err) }
//	defer p.Close()
//	state, _ := p.State(eph.Mars, t)
//
//	// Multi-kernel (deep historical + modern)
//	p, err := eph.NewProvider(ctx, eph.Planets, "de441_part-1", eph.WithKernel("de441_part-2"))
//
//	// NORAD satellite (ISS)
//	sat, err := eph.NewProvider(ctx, eph.Satellites, "ISS",
//	    eph.WithTLE(line1, line2))
//
// # Default Provider
//
// The [Default] provider uses algorithms from the IAU SOFA library (via gofaext):
//   - Sun: IAU 2000 Earth ephemeris (Epv00), providing ~0.1″ solar accuracy.
//   - Moon: Meeus 1998 algorithm (Moon98), providing geocentric GCRS-like state.
//
// # Choosing a Provider
//
// [Default] needs no download and no [context.Context] consent step, at the
// cost of accuracy. A JPL kernel via [NewProvider] is sub-arcsecond (validated
// against JPL Horizons, see docs/VALIDATION.md) but requires a one-time,
// consent-gated download (see the remote package doc for the consent gate).
// de440s/de440/de442/de441 do not differ from each other in accuracy — only
// in time-span coverage and file size; the real accuracy jump is
// [Default] versus any JPL kernel:
//
//	Provider                    Accuracy                    Size         Download
//	Default()                   ~0.1″ solar (Sun); Moon      built-in     not needed
//	                             via Meeus 1998, not JPL-grade
//	NewProvider(…, "de440s")     sub-arcsecond vs. Horizons   ~32 MB       one-time
//	NewProvider(…, "de440")      same fidelity, wider span    ~115 MB      one-time
//	NewProvider(…, "de442")      same fidelity, wider span    ~115 MB      one-time
//	NewProvider(…, "de441_…")    same fidelity, millennia     multi-GB/part one-time
//
// Default to de440s once JPL-grade positions are needed; reach for de440/de442
// only when a request spans centuries beyond de440s's coverage, and de441
// only for millennia-scale work.
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
