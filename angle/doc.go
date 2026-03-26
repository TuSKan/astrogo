// Package angle implements the [Angle] type, a lightweight value type for
// representing angular quantities in astrogo.
//
// # Why a dedicated Angle type?
//
// astrogo also provides the `quantity` package for generic dimensioned scalars.
// Angle is kept separate for three reasons:
//
//  1. Hot-path performance: Angle is a plain float64 alias. No interface boxing,
//     no per-call dispatch, no allocations. Trig functions and normalization are
//     single-instruction paths.
//
//  2. Astronomy-specific semantics: the Angle type encodes the [0, 2π) and
//     (-π, π] wrapping conventions, HMS/DMS formatting, and sexagesimal parsing
//     that have no natural home in a generic quantity package.
//
//  3. Compile-time type safety: accepting Angle instead of float64 in coordinate
//     and frame APIs makes it impossible to pass a raw degree value where radians
//     are expected.
//
// # Canonical representation
//
// Internally Angle is stored in radians. No value is ever silently normalized
// on construction; callers who need wrapping use [Angle.Wrap2Pi] or
// [Angle.WrapPi] explicitly.
//
// # Dependencies
//
// This package depends only on the Go standard library. It does not import
// `constants`, `units`, or `quantity`.
package angle
