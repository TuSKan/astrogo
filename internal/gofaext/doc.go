// Package gofaext wraps the github.com/hebl/gofa package for use within astrogo.
//
// gofaext is an internal adapter layer. It exists to:
//   - isolate astrogo packages from gofa's calling conventions and type naming,
//   - provide thin helpers that translate astrogo value types (Julian dates,
//     ra/dec in radians, …) into the argument shapes gofa routines expect, and
//   - centralise any workaround needed for gofa edge cases so the rest of the
//     codebase stays clean.
//
// Nothing in this package is part of the public API. Callers outside astrogo
// must not import it.
//
// # SOFA attribution
//
// This package uses routines and computations derived from the [github.com/hebl/gofa]
// module, which is itself a Go port of routines from the International
// Astronomical Union's [Standards Of Fundamental Astronomy (SOFA)] software.
// astrogo (via gofa) uses SOFA algorithms under license to gofa's author but
// does not itself constitute software provided by and/or endorsed by SOFA.
// gofaext's function names (Apco13, Atciq, Gst06a, …) mirror gofa's own
// naming, which already omits the "iau"/"sofa" prefixes SOFA's own routines
// use. See the top-level NOTICE file for the full attribution statement.
//
// [Standards Of Fundamental Astronomy (SOFA)]: http://www.iausofa.org
package gofaext
