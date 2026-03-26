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
package gofaext
