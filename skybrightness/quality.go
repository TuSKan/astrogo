package skybrightness

import "strings"

// Flag records how a prediction was actually constrained, so a caller can
// tell a result driven by current observatory measurements from one driven
// mostly by climatology.
//
// These are structured flags rather than log lines because the distinction
// is part of the answer, not a diagnostic: a scheduler may accept a
// climatological aerosol but refuse a climatological cloud field, and it
// can only do that if the result says which it got.
type Flag uint32

// Quality flags. Each names what a specific input was, not how good the
// result is — "good" is a judgement the caller makes from these.
const (
	// MeasuredAtmosphere marks surface conditions from real measurements.
	MeasuredAtmosphere Flag = 1 << iota
	// ClimatologicalAtmosphere marks surface conditions from climatology.
	ClimatologicalAtmosphere
	// MeasuredAerosol marks aerosol properties from measurement (AERONET,
	// a photometer) rather than assumption.
	MeasuredAerosol
	// ClimatologicalAerosol marks assumed or climatological aerosol.
	ClimatologicalAerosol
	// MeasuredCloud marks an observed cloud field.
	MeasuredCloud
	// ForecastCloud marks a forecast cloud field.
	ForecastCloud
	// UnknownCloud marks a cloud state that was not supplied at all.
	UnknownCloud
	// MeasuredAirglow marks airglow constrained by recent local data.
	MeasuredAirglow
	// SolarAdjustedAirglow marks airglow scaled by a solar-activity index.
	SolarAdjustedAirglow
	// ClimatologicalAirglow marks airglow from climatology alone.
	ClimatologicalAirglow
	// MeasuredSourceSpectrum marks an artificial-source spectral power
	// distribution taken from a real inventory.
	MeasuredSourceSpectrum
	// AssumedSourceSpectrum marks an assumed source spectrum — satellite
	// radiance alone cannot determine one.
	AssumedSourceSpectrum
	// MeasuredEmissionFunction marks a measured upward emission function.
	MeasuredEmissionFunction
	// AssumedEmissionFunction marks an assumed upward emission function.
	AssumedEmissionFunction
	// PrecomputedRT marks radiance taken from a precomputed
	// radiative-transfer product rather than evaluated natively.
	PrecomputedRT
	// ExtrapolatedModel marks evaluation outside a component's stated
	// validity domain.
	ExtrapolatedModel
	// ApproximateMultipleScattering marks radiance whose higher scattering
	// orders come from an empirical broadband factor rather than a
	// radiative-transfer solution. It is closer than single scattering
	// alone, and it is still a fit made at one site.
	ApproximateMultipleScattering
	// NoComponents marks a model with nothing registered — the Phase 0
	// state. The radiance is identically zero and is not a sky prediction.
	NoComponents
)

// flagNames pairs each flag with its name, in declaration order.
//
//nolint:gochecknoglobals // lookup table for String
var flagNames = []struct {
	flag Flag
	name string
}{
	{MeasuredAtmosphere, "MeasuredAtmosphere"},
	{ClimatologicalAtmosphere, "ClimatologicalAtmosphere"},
	{MeasuredAerosol, "MeasuredAerosol"},
	{ClimatologicalAerosol, "ClimatologicalAerosol"},
	{MeasuredCloud, "MeasuredCloud"},
	{ForecastCloud, "ForecastCloud"},
	{UnknownCloud, "UnknownCloud"},
	{MeasuredAirglow, "MeasuredAirglow"},
	{SolarAdjustedAirglow, "SolarAdjustedAirglow"},
	{ClimatologicalAirglow, "ClimatologicalAirglow"},
	{MeasuredSourceSpectrum, "MeasuredSourceSpectrum"},
	{AssumedSourceSpectrum, "AssumedSourceSpectrum"},
	{MeasuredEmissionFunction, "MeasuredEmissionFunction"},
	{AssumedEmissionFunction, "AssumedEmissionFunction"},
	{PrecomputedRT, "PrecomputedRT"},
	{ExtrapolatedModel, "ExtrapolatedModel"},
	{ApproximateMultipleScattering, "ApproximateMultipleScattering"},
	{NoComponents, "NoComponents"},
}

// Has reports whether every flag in want is set.
func (f Flag) Has(want Flag) bool { return f&want == want }

// String lists the set flags, pipe-separated.
func (f Flag) String() string {
	if f == 0 {
		return "none"
	}

	var set []string

	for _, fn := range flagNames {
		if f&fn.flag != 0 {
			set = append(set, fn.name)
		}
	}

	return strings.Join(set, "|")
}

// Quality is the set of flags describing one Estimate.
type Quality struct {
	Flags Flag
}

// Add sets flags.
func (q *Quality) Add(f Flag) { q.Flags |= f }

// Has reports whether every flag in want is set.
func (q Quality) Has(want Flag) bool { return q.Flags.Has(want) }

// String renders the flag set.
func (q Quality) String() string { return q.Flags.String() }
