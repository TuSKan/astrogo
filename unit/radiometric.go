package unit

// Radiometric SI units, added for skybrightness's spectral sky-radiance
// engine (see docs/skybrightness.md §3). These exist for documentation,
// provenance serialization, and boundary parsing — never for the numeric
// hot path, where skybrightness uses its own bespoke float64 types
// instead. See the package doc's "Radiometric type safety" section for why.
var (
	// Watt is W = kg·m²·s⁻³ (power).
	Watt = Unit{Dimension: Power, ScaleFactor: 1, Name: "watt", Symbol: "W"}

	// Joule is J = kg·m²·s⁻² (energy).
	Joule = Unit{Dimension: Energy, ScaleFactor: 1, Name: "joule", Symbol: "J"}

	// Hertz is Hz = s⁻¹ (frequency).
	Hertz = Unit{Dimension: Time.PowInt(-1), ScaleFactor: 1, Name: "hertz", Symbol: "Hz"}

	// Nanometre is nm = 1e-9 m — the wavelength unit spectral radiance is
	// expressed per.
	Nanometre = Unit{Dimension: Length, ScaleFactor: 1e-9, Name: "nanometre", Symbol: "nm"}

	// Candela is cd (luminous intensity) — the base for Luminance below.
	Candela = Unit{Dimension: Luminosity, ScaleFactor: 1, Name: "candela", Symbol: "cd"}

	// Steradian is sr (solid angle).
	//
	// NOTE: Steradian is dimensionally identical to [One], exactly as
	// [Radian] is — Dimension has no tag distinguishing a solid angle from
	// a bare dimensionless ratio (see the package doc). Adding Steradian
	// gives real documentation and serialization value but NO type-level
	// protection against a radiance silently cancelling into an
	// irradiance; that protection lives in skybrightness's own named
	// scalar types, not here. Do not rely on Compatible/ConversionFactor
	// to catch a Steradian-vs-One mix-up — it can't.
	Steradian = Unit{Dimension: Dimensionless, ScaleFactor: 1, Name: "steradian", Symbol: "sr"}
)

// Composed radiometric units (unit.Unit values, for documentation,
// provenance serialization, and boundary parsing). Named with a "Unit"
// suffix specifically to avoid colliding with the zero-cost quantity
// TYPES of nearly the same name declared in quantity_types.go
// (unit.Irradiance, unit.Radiance, ...) — those are what application code
// actually computes with; these unit.Unit values exist only to describe
// them. Each is built through the same Mul/Div composition every other
// composite unit in this codebase uses (see constants/units.go), so a
// wrong exponent is a compile-time-visible composition error, not a
// hand-typed literal.
var (
	// IrradianceUnit is W·m⁻².
	IrradianceUnit = Watt.Div(Meter.PowInt(2))

	// RadianceUnit is W·m⁻²·sr⁻¹. Dimensionally identical to
	// IrradianceUnit — see the Steradian note above; do not use this fact
	// to justify treating the two as interchangeable in application code
	// (use the distinct unit.Radiance/unit.Irradiance TYPES for that).
	RadianceUnit = IrradianceUnit.Div(Steradian)

	// SpectralRadianceUnit is W·m⁻²·sr⁻¹·nm⁻¹ — the primary quantity of
	// the skybrightness spectral engine, L_λ(λ, altitude, azimuth, site,
	// epoch).
	SpectralRadianceUnit = RadianceUnit.Div(Nanometre)

	// SpectralIrradianceUnit is W·m⁻²·nm⁻¹.
	SpectralIrradianceUnit = IrradianceUnit.Div(Nanometre)

	// LuminanceUnit is cd·m⁻².
	LuminanceUnit = Candela.Div(Meter.PowInt(2))
)
