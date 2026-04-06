// Package plan provides high-level tools for astronomical observation planning
// and robust scheduling.
//
// It includes the core observability evaluation API, a high-precision
// event-based solver for identifying rise, set, and transit times, and a
// full scheduling engine for generating observation timelines subject to
// constraints, slew transitions, and dynamical block prioritizations.
//
// # Refinement
//
// The event finder uses a two-stage approach: a coarse sampling pass followed
// by numerical refinement using bisection (for roots) and golden-section search
// (for local maxima). This provides high temporal accuracy (e.g., 1 second)
// without requiring analytical closed-form solutions.
package plan
