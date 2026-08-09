package skybrightness

import "strings"

// QualityFlags is a bitset of caveats attached to a Component's or
// Result's output. QualityFlagOK (the zero value) means no caveat applies.
type QualityFlags uint64

const (
	// QualityFlagOK is the zero value: no caveat applies.
	QualityFlagOK QualityFlags = 0

	// QualityFlagLegacyPhysics marks output produced by a Legacy* fast
	// model (ModeLegacy) rather than the full spectral physics.
	QualityFlagLegacyPhysics QualityFlags = 1 << iota

	// QualityFlagPassbandTruncated marks a passband integration where the
	// spectral grid did not fully cover the passband's response range.
	QualityFlagPassbandTruncated

	// QualityFlagOutOfSurrogateDomain marks a request whose atmospheric
	// state fell outside a surrogate model's trained domain (Phase 6).
	QualityFlagOutOfSurrogateDomain

	// QualityFlagStaleAtmosphere marks an AtmosphereState older than
	// EvaluationOptions.MaxInputAge.
	QualityFlagStaleAtmosphere

	// QualityFlagFallbackApplied marks a Result produced after an
	// explicit, recorded mode fallback (see Provenance.Fallbacks).
	QualityFlagFallbackApplied

	// QualityFlagNoVegaZeroPoint marks a Vega-system output request
	// against a Passband with no VegaZeroPoint.
	QualityFlagNoVegaZeroPoint

	// QualityFlagTwilightExtrapolated marks output computed by
	// extrapolating a twilight model beyond its validated solar
	// depression range.
	QualityFlagTwilightExtrapolated

	// QualityFlagHorizonBlocked marks a direction/source pair where a
	// horizon profile blocked a direct contribution.
	QualityFlagHorizonBlocked

	// QualityFlagCloudUncalibrated marks output from
	// rt.FastCloudApproximation (Phase 5) rather than the full physical
	// cloudy-sky model.
	QualityFlagCloudUncalibrated

	// QualityFlagSourceDataMasked marks a request where an emission-field
	// pixel was masked (snow/cloud/lunar contamination, or otherwise
	// flagged Poor/Missing by its dataset) rather than silently
	// interpolated.
	QualityFlagSourceDataMasked

	// QualityFlagSingleScatteringOnly marks rt.ClearSkyPhysical output
	// computed with higher scattering orders disabled.
	QualityFlagSingleScatteringOnly

	// QualityFlagBelowHorizon marks a direction at or below the local
	// horizon.
	QualityFlagBelowHorizon
)

var qualityFlagNames = map[QualityFlags]string{
	QualityFlagLegacyPhysics:        "LegacyPhysics",
	QualityFlagPassbandTruncated:    "PassbandTruncated",
	QualityFlagOutOfSurrogateDomain: "OutOfSurrogateDomain",
	QualityFlagStaleAtmosphere:      "StaleAtmosphere",
	QualityFlagFallbackApplied:      "FallbackApplied",
	QualityFlagNoVegaZeroPoint:      "NoVegaZeroPoint",
	QualityFlagTwilightExtrapolated: "TwilightExtrapolated",
	QualityFlagHorizonBlocked:       "HorizonBlocked",
	QualityFlagCloudUncalibrated:    "CloudUncalibrated",
	QualityFlagSourceDataMasked:     "SourceDataMasked",
	QualityFlagSingleScatteringOnly: "SingleScatteringOnly",
	QualityFlagBelowHorizon:         "BelowHorizon",
}

// Has reports whether every bit set in f is also set in q.
func (q QualityFlags) Has(f QualityFlags) bool { return q&f == f }

// Strings returns the set flags' names, in ascending bit order. Returns
// nil for QualityFlagOK.
func (q QualityFlags) Strings() []string {
	if q == QualityFlagOK {
		return nil
	}

	var out []string

	for bit := QualityFlags(1); bit != 0; bit <<= 1 {
		if q.Has(bit) {
			if name, ok := qualityFlagNames[bit]; ok {
				out = append(out, name)
			}
		}
	}

	return out
}

// String implements fmt.Stringer.
func (q QualityFlags) String() string {
	if q == QualityFlagOK {
		return "OK"
	}

	return strings.Join(q.Strings(), "|")
}
