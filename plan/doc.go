// Package plan provides high-level tools for astronomical observation planning
// and robust scheduling.
//
// It includes the core observability evaluation API, a high-precision
// generalized EventSolver for identifying rise, set, and transit times, as well
// as complex geometric events (Conjunctions, Oppositions, and Greatest Elongations).
// It also features a full scheduling engine for generating observation timelines
// subject to constraints, slew transitions, and dynamical block prioritizations.
//
// # Refinement
//
// The EventSolver uses a two-stage approach: a coarse sampling pass followed
// by numerical refinement using bisection (for zero-crossing boundaries) and
// golden-section search (for local maxima/extrema). This provides high temporal
// accuracy (e.g., 1 second) without requiring analytical closed-form solutions.
package plan
