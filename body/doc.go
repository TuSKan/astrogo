// Package body represents celestial bodies and their categories.
//
// # Design
//
// This package provides a shared domain model for identifying and
// categorizing celestial bodies (Stars, Planets, Moons, etc.). It is
// intentionally lightweight and does not contain ephemeris calculation
// logic, which resides in the `ephemeris` package.
//
// # Identifiers
//
// Bodies are identified by a unique [ID]. For major Solar System bodies,
// the [ID] values are compatible with standard ephemeris indices (e.g.,
// [Sun], [Moon], [Mars]).
package body
