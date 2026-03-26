// Package target defines the unified observation target abstraction for astrogo.
//
// It provides a common interface for representing anything that can appear on the sky,
// including fixed celestial objects from catalogs, moving solar system bodies from
// ephemeris, and custom user-defined coordinates.
//
// Moving targets (e.g., planets) return coordinates that depend on the evaluation
// time. Note that apparent or topocentric corrections are not implied by this
// package unless explicitly stated.
//
// This package serves as a bridge between catalog, ephemeris, sky, and planning packages.
package target
