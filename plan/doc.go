// Package plan provides high-level observation planning and scheduling.
//
// # Design
//
// Planning in astrogo is organized around the [Planner], which evaluates
// potential [Observation] targets against a set of [constraint.Constraint]
// objects for a specific [observatory.Site].
//
// This package is the entry point for determining which targets are
// observable and in what order they should be prioritized based on
// scientific or geometric criteria (e.g., peak altitude).
//
// # Future Work
//
// This first stable version provides basic filtering and ranking. Future
// iterations will include automated schedule block optimization,
// calibration insertion, and greedy or heuristic-based schedulers.
package plan
