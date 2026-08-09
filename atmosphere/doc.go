// Package atmosphere provides atmospheric refraction models, observational
// metrics, and a richer atmospheric-state type for astronomical
// observations.
//
// [Refraction] is the small, freely-literal-constructed environmental
// profile (pressure, temperature, humidity, wavelength) consumed by the
// pluggable [RefractionModel] interface and its three concrete
// implementations:
//
//   - [RefractionNone]          — disables refraction entirely.
//   - [RefractionApproximate]   — Saemundsson/Bennett tangent formula (~0.1 arcmin above 15°).
//   - [RefractionRigorous]      — Saemundsson (1986) / Bennett (1982) with pressure, temperature,
//     humidity, and wavelength corrections.
//
// The package also provides the [Airmass] function (Pickering 2002) and the
// [ZenithDistance] helper.
//
// [Atmosphere] is a separate, richer, immutable, Builder-validated type:
// surface pressure/temperature (via an embedded [Refraction]), aerosol,
// clouds, ozone, precipitable water, surface reflectance, terrain horizon,
// and provenance — general-purpose atmospheric state, not specific to any
// one consumer. It composes a [Refraction] as its own surface-conditions
// field rather than duplicating pressure/temperature: [Atmosphere.Surface]
// returns Kelvin (skybrightness's native convention) and
// [Atmosphere.Refraction] returns the embedded [Refraction] value directly
// (Celsius, matching every other RefractionModel call site in this
// package) — the one explicit unit conversion this design needs happens at
// that single boundary, in [Builder.Surface].
//
// Renaming note: [Atmosphere] and [Refraction] swapped names in this
// release. The type now called [Refraction] used to be exported as
// Atmosphere; the type now called [Atmosphere] used to be exported as
// State. This was a deliberate, same-release hard break with no
// deprecation alias — Go cannot alias one identifier to two different
// meanings at once, so freeing "Atmosphere" for the richer type
// necessarily retired the refraction struct's old name immediately, not
// over a deprecation cycle.
//
// [StandardDefault] is the zero-aerosol reference constructor.
// [RuralAerosol], [UrbanAerosol], [DesertAerosol], and [MaritimeAerosol]
// seed a [Builder] with a real, named, published aerosol-type optical
// model (Hess, Koepke & Schult 1998's OPAC dataset) instead — single-
// scattering albedo, asymmetry parameter, and Angstrom exponent come
// from that source; aerosol optical depth stays a caller-supplied,
// real-time-varying parameter, never hardcoded. Each returns a *Builder,
// not a terminal *Atmosphere, so a caller can chain further
// customization (PrecipitableWater, SurfaceAlbedo, Source, ...) before
// Build().
package atmosphere
