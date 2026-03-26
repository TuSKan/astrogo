// Package coord provides concrete value types for astronomical coordinate systems.
//
// # Design
//
// Unlike systems that use a single dynamic "SkyCoord" abstraction, `astrogo`
// provides concrete, named types for each coordinate frame (e.g., [ICRS],
// [AltAz], [Galactic]).
//
// This approach offers several benefits:
//   - Semantic Clarity: A function signature clearly states whether it expects
//     equatorial or horizontal coordinates.
//   - Performance: No interface dispatch, dynamic field lookups, or internal
//     lazy-transformation state is required for basic coordinate storage.
//   - Dimension-Safety: Each type includes an optional `Dist` (distance) field
//     for 3D positioning.
//
// # Transformations
//
// While this package defines the storage types, the logic for converting
// between them lives in the `transform` and `frame` packages. This separation
// keeps the coordinate primitives lightweight and dependency-free.
package coord
