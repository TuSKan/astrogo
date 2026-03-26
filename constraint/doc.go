// Package constraint provides composable observation constraints for planning.
//
// # Design
//
// Constraints are small, atomic objects that evaluate whether a [sky.Object]
// satisfies a particular observing condition (e.g., minimum altitude,
// maximum airmass) at a given site and time.
//
// Multiple constraints can be combined using [EvaluateAll] to determine if
// an object is observable under a set of requirements.
//
// # Extensibility
//
// The [Constraint] interface is intentionally minimal, allowing for future
// implementations of twilight, moon separation, and weather constraints.
package constraint
