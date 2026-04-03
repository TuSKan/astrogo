// Package plan provides high-level tools for astronomical observation planning.
//
// It includes the core observability evaluation API and a high-precision
// event-based solver for identifying rise, set, and transit times for
// both fixed and moving celestial targets.
//
// # Refinement
//
// The event finder uses a two-stage approach: a coarse sampling pass followed
// by numerical refinement using bisection (for roots) and golden-section search
// (for local maxima). This provides high temporal accuracy (e.g., 1 second)
// without requiring analytical closed-form solutions.
package plan
