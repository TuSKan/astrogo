// Package visibility provides practical visibility and time-window information.
//
// # Design
//
// This package builds on top of `sky`, `observatory`, and `time` to answer
// practical observing questions:
//  - "Is this object above my horizon right now?"
//  - "When is this object observable tonight?"
//  - "When does this object reach its maximum altitude?"
//
// # Accuracy and Performance
//
// Root-finding for exact rise/set/transit times is intensive. For v1, this
// package uses a sampled grid-search approach. Accuracy is directly
// proportional to the step size used in searches. For sub-second precision,
// consider using specialized ephemeris providers or future iterative
// refinement tools.
package visibility
