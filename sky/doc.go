// Package sky provides high-level astronomical utility functions.
//
// # Design
//
// The package distinguishes between two main types of quantities:
//
//  1. Geometric Quantities: These depend only on the relative positions of
//     objects on the celestial sphere (e.g., [Separation], [PositionAngle]).
//  2. Observer-Dependent Quantities: These depend on the observer's location
//     on Earth and the time of observation (e.g., [AltAz], [Airmass]).
//
// # Performance
//
// Coordinate calculations in this package are optimized for clarity and
// correctness. For large-scale batch processing, consider using vectorized
// operations in the `transform` package directly.
package sky
