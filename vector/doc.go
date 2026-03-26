// Package vector provides a minimal 3-D Cartesian geometry kernel for astrogo.
//
// # Scope
//
// This package intentionally provides only the primitives needed by astrogo's
// coordinate and frame layers:
//
//   - [Vec3]: a 3-component Cartesian vector with arithmetic, norm, and unit-vector operations.
//   - [FromSpherical] / [Vec3.ToSpherical]: conversion between Cartesian and spherical coords.
//   - [Vec3.RotateX], [Vec3.RotateY], [Vec3.RotateZ]: in-place rotation about the principal axes.
//
// It is NOT a general linear algebra library. There is no matrix type, no
// eigendecomposition, no LU factorisation. If a caller needs to compose
// multiple rotations, they can chain the Rotate methods or accumulate the
// product in their own code.
//
// # Design
//
// All operations return new Vec3 values (no mutation). Every function is a
// candidate for compiler inlining: no interfaces, no generics, no heap
// allocations. Callers in hot coordinate-transform paths can rely on these
// primitives being as cheap as equivalent raw float64 arithmetic.
//
// # Dependencies
//
// Standard library only (math). No intra-module imports.
package vector
