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
		ScaleFactor: 1, Dimension: unit.DimVelocity,
	}
	// radianPerSecond is rad·s⁻¹ (angular velocity) — WGS84.AngularVelocity.
	//
	// A radian is dimensionless, so this carries the dimension of an
	// inverse time; the symbol is what tells a reader it is a rotation
	// rate rather than a frequency.
	radianPerSecond = unit.Unit{
		Name: "radian per second", Symbol: "rad/s",
		ScaleFactor: 1, Dimension: unit.Dimension{T: -1},
	}
	// squareMeter is m² (area) — ThomsonCrossSection.
	squareMeter = unit.Unit{
		Name: "square meter", Symbol: "m²",
		ScaleFactor: 1, Dimension: unit.DimArea,
	}
	// jouleSecond is J·s = kg·m²·s⁻¹ (action) — PlanckConstant.
	jouleSecond = unit.Unit{
		Name: "joule second", Symbol: "J·s",
		ScaleFactor: 1, Dimension: unit.DimEnergy.Mul(unit.DimTime),
	}
	// joulePerKelvin is J·K⁻¹ = kg·m²·s⁻²·K⁻¹ (entropy) — BoltzmannConstant.
	joulePerKelvin = unit.Unit{
		Name: "joule per kelvin", Symbol: "J/K",
		ScaleFactor: 1, Dimension: unit.DimEnergy.Div(unit.DimTemperature),
	}
	// cubicMeterPerKilogramSecondSquared is m³·kg⁻¹·s⁻² — GravitationalConstant.
	cubicMeterPerKilogramSecondSquared = unit.Unit{
		Name: "cubic meter per kilogram second squared", Symbol: "m³/(kg·s²)",
		ScaleFactor: 1, Dimension: unit.DimVolume.Div(unit.DimMass).Div(unit.DimTime.PowInt(2)),
	}
	// cubicMeterPerSecondSquared is m³·s⁻² — a standard gravitational
	// parameter GM (mass already folded in, unlike G alone) —
	// SunGravitationalParameter.
	cubicMeterPerSecondSquared = unit.Unit{
		Name: "cubic meter per second squared", Symbol: "m³/s²",
		ScaleFactor: 1, Dimension: unit.DimVolume.Div(unit.DimTime.PowInt(2)),
	}
	// wattPerSquareMeterHertz is W·m⁻²·Hz⁻¹ (spectral flux density, SI
	// base of the jansky) — PhotometricSet.ABZeroPoint.
	wattPerSquareMeterHertz = unit.Unit{
		Name: "watt per square meter hertz", Symbol: "W/(m²·Hz)",
		ScaleFactor: 1, Dimension: unit.DimSpectralFlux,
	}
	// wattPerSquareMeterKelvin4 is W·m⁻²·K⁻⁴ — StefanBoltzmannConstant.
	wattPerSquareMeterKelvin4 = unit.Unit{
		Name: "watt per square meter kelvin4", Symbol: "W/(m²·K⁴)",
		ScaleFactor: 1, Dimension: unit.DimPower.Div(unit.DimArea).Div(unit.DimTemperature.PowInt(4)),
	}
)
