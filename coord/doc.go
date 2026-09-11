// Package coord provides concrete value types for astronomical coordinate systems
// and high-precision topocentric transformations.
//
// # Design
//
// Unlike systems that use a single dynamic "SkyCoord" abstraction, astrogo
// provides concrete, named types for each coordinate frame (e.g., [ICRS],
// [AltAz], [Galactic], [Ecliptic], [Astrometric], [Apparent]).
//
// This approach offers several benefits:
//   - Semantic Clarity: A function signature clearly states whether it expects
//     equatorial or horizontal coordinates.
//   - Performance: No interface dispatch, dynamic field lookups, or internal
//     lazy-transformation state is required for basic coordinate storage.
//   - Dimension-Safety: Each type includes an optional Dist (distance) field
//     for 3D positioning.
//
// # Transformations
//
// The [Context] type precomputes expensive SOFA intermediate astrometry
// parameters (ASTROM) once per observation epoch, then amortises the cost
// across many targets. It supports the full pipeline:
//
//	Geometric → Astrometric → Apparent → Observed (Alt/Az)
//
// Pure frame rotations ([ICRSToGalactic], [ICRSToEcliptic]) are available as
// standalone functions.
//
// The [Reducer] provides a lightweight topocentric reduction pipeline that
// also handles chromatic atmospheric dispersion.
//
// # Offsets from a target
//
// [SkyOffset] is a frame centred on a chosen position, so nearby objects are
// described by how far they are from it rather than by absolute coordinates —
// dither and mosaic patterns, offset guide stars, slit layouts, finder charts.
// It is a rotation of the sphere rather than a projection onto a plane, which
// is what separates it from subtracting coordinates: at δ = 80° a point one
// degree due east of a target differs from it by 5.7° of right ascension and
// by three arcminutes of declination.
//
// # Earth Orientation
//
// Both [Context] and [Reducer] query the global IERS EOP model for DUT1 and
// polar motion (XP/YP). If IERS data is unavailable, a one-time log warning
// is emitted and zero corrections are applied (UT1 ≈ UTC, ~0.9 s worst case).
//
// # Concurrency
//
// A [Context] is read-only once built: no method assigns to its fields, and
// [Context.Clone] and [Context.AtTime] return new values rather than mutating
// the receiver. One Context may therefore be shared across goroutines, which
// is what makes the "build one per epoch and reuse it" rule practical — the
// expensive SOFA matrix is computed once and read concurrently.
//
// Everything else here is a value type.
package coord
