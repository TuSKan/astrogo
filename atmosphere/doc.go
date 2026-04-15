// Package atmosphere provides atmospheric refraction models and observational
// metrics for astronomical observations.
//
// It defines the [Atmosphere] environmental profile, the pluggable
// [RefractionModel] interface, and three concrete implementations:
//
//   - [RefractionNone]          — disables refraction entirely.
//   - [RefractionApproximate]   — Saemundsson/Bennett tangent formula (~0.1 arcmin above 15°).
//   - [RefractionRigorous]      — Saemundsson (1986) / Bennett (1982) with pressure, temperature,
//     humidity, and wavelength corrections.
//
// The package also provides the [Airmass] function (Pickering 2002) and the
// [ZenithDistance] helper.
package atmosphere
