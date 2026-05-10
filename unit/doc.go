// Package unit provides a representation of scalar physical quantities
// with explicit units.
//
// # Design
//
// A [Quantity] is a pair of a numerical value and a [units.Unit].
//
// This package is for general-purpose scientific calculations where explicit
// unit handling and dimension-safety are required.
//
// # Distinction from angle.Angle
//
// While an angle is technically a quantity (dimensionless), astrogo provides
// a specialized `angle.Angle` type. This distinction is intentional:
//   - Performance: `angle.Angle` is a top-level `float64` alias, allowing for
//     extremely fast trigonometry without struct overhead or unit lookups.
//   - Semantics: Angles in astronomy require specialized normalization
//     (e.g., [0, 2π), [-π/2, π/2]) and sexagesimal formatting (DMS/HMS) that
//     do not apply to general physical quantities like mass or pressure.
//
// Use [Quantity] for general physics (lengths, masses, times) and
// `angle.Angle` for coordinate geometry and telescope pointing.
package unit
