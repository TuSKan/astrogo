// Package units provides a dimension-aware system for physical units and
// conversions.
//
// # Design
//
// The package is built on two core types:
//   - [Dimension]: Represents physical dimensions (Length, Mass, Time, etc.)
//     using integer exponents.
//   - [Unit]: Represents a specific measurement scale (e.g., Meter, Second)
//     with a dimension and a scale factor relative to SI base units.
//
// This design allows for:
//   - Type-safe conversions: You cannot convert Meters to Seconds.
//   - Derived units: Multiplying a Length unit by a Length unit automatically
//     produces an Area unit.
//   - Explicit scale factors: All conversions are relative to a canonical
//     SI base (meters, kilograms, seconds, radians, kelvin).
//
// # Scope
//
// This package owns physical unit definitions and conversion factors. It does
// NOT own:
//   - Generic quantities (value + unit pairs); those live in the `quantity` package.
//   - Astronomical time-scales (UTC, TAI, TT); those live in the `time` package.
//   - Coordinate-specific angular types; those live in the `angle` package.
//
// # Performance
//
// Units and Dimensions are small value types. Comparison and basic algebra
// are fast and allocation-free.
package units
