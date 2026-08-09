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
//
// # Radiometric type safety
//
// [Steradian] is dimensionally identical to [One], exactly as [Radian] is —
// [Dimension] has only the seven SI base exponents and no tag distinguishing
// a solid angle from a bare dimensionless ratio, so [RadianceUnit] and
// [IrradianceUnit] (the unit.Unit VALUES, used for documentation and
// provenance serialization) compare Compatible and even Equals in dimension
// despite being physically distinct quantities. This is deliberate, not an
// oversight: unit.Dimension's runtime model cannot and does not protect a
// caller from cancelling a radiance into an irradiance.
//
// Real radiometric type safety instead comes from the zero-cost quantity
// TYPES declared in quantity_types.go — [Radiance], [Irradiance],
// [SpectralRadiance], [LuminanceCdM2], and their neighbors — each a
// distinct named float64 type the Go compiler itself keeps apart, the same
// way `angle.Angle` is kept apart from a bare float64 meant for something
// else. These live directly in this package (not duplicated into a
// consumer package) specifically so a hot numeric loop — e.g.
// skybrightness's spectral sky-radiance engine, evaluating ~10^4 directions
// x ~10^2-10^3 wavelengths per call — can use them with zero struct
// overhead, unlike [Quantity]. See docs/skybrightness.md §3 for the full
// design rationale.
package unit
