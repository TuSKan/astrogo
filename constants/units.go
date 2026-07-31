package constants

import "github.com/TuSKan/astrogo/unit"

// Composite SI units the constants in this package are expressed in. Each
// has ScaleFactor 1, so every Constant.Value is an SI base-unit value; the
// dimensions are composed through unit's own algebra rather than written
// as raw exponent literals, so a wrong exponent is a compile-time-visible
// composition error rather than a silent typo.
//
// Kept unexported: these exist to give Constant.Unit a correct dimension
// and a readable symbol, not to extend unit's public vocabulary. Spelling
// follows unit/units.go ("meter", not "metre").
var (
	// meterPerSecond is m·s⁻¹ (velocity) — SpeedOfLight.
	meterPerSecond = unit.Unit{
		Name: "meter per second", Symbol: "m/s",
		ScaleFactor: 1, Dimension: unit.Velocity,
	}
	// squareMeter is m² (area) — ThomsonCrossSection.
	squareMeter = unit.Unit{
		Name: "square meter", Symbol: "m²",
		ScaleFactor: 1, Dimension: unit.Area,
	}
	// jouleSecond is J·s = kg·m²·s⁻¹ (action) — PlanckConstant.
	jouleSecond = unit.Unit{
		Name: "joule second", Symbol: "J·s",
		ScaleFactor: 1, Dimension: unit.Energy.Mul(unit.Time),
	}
	// joulePerKelvin is J·K⁻¹ = kg·m²·s⁻²·K⁻¹ (entropy) — BoltzmannConstant.
	joulePerKelvin = unit.Unit{
		Name: "joule per kelvin", Symbol: "J/K",
		ScaleFactor: 1, Dimension: unit.Energy.Div(unit.Temperature),
	}
	// cubicMeterPerKilogramSecondSquared is m³·kg⁻¹·s⁻² — GravitationalConstant.
	cubicMeterPerKilogramSecondSquared = unit.Unit{
		Name: "cubic meter per kilogram second squared", Symbol: "m³/(kg·s²)",
		ScaleFactor: 1, Dimension: unit.Volume.Div(unit.Mass).Div(unit.Time.PowInt(2)),
	}
	// cubicMeterPerSecondSquared is m³·s⁻² — a standard gravitational
	// parameter GM (mass already folded in, unlike G alone) —
	// SunGravitationalParameter.
	cubicMeterPerSecondSquared = unit.Unit{
		Name: "cubic meter per second squared", Symbol: "m³/s²",
		ScaleFactor: 1, Dimension: unit.Volume.Div(unit.Time.PowInt(2)),
	}
)
