package skybrightness

// Mode names an operating mode for a spectral evaluation. Modes never fall
// back into one another silently — see EvaluationOptions.Fallback and
// Provenance.Fallbacks (docs/skybrightness.md §7).
type Mode uint8

const (
	// ModeClimatology uses a versioned, deterministic, fully offline
	// baseline atmospheric/emission state.
	ModeClimatology Mode = iota

	// ModeHistorical uses dataset-backed state for a specified past epoch.
	ModeHistorical

	// ModeNowcast uses a recently acquired state; Provenance.Atmosphere
	// carries its issue time and age.
	ModeNowcast

	// ModeForecast uses a forecast state; Provenance.Atmosphere carries
	// its initialization time, lead time, and ensemble uncertainty.
	ModeForecast

	// ModeUserSupplied means the caller provided every physical state
	// (AtmosphereState, emission field, ...) directly.
	ModeUserSupplied

	// ModeFast selects the v1-equivalent empirical physics —
	// natural.ConstantAirglow, natural.KrisciunasSchaeferMoonlight,
	// SchaeferNELM — re-implemented against the new spectral API, not a
	// compatibility shim for the old package. See docs/skybrightness.md
	// §15.
	ModeFast
)

// String implements fmt.Stringer.
func (m Mode) String() string {
	switch m {
	case ModeClimatology:
		return "Climatology"
	case ModeHistorical:
		return "Historical"
	case ModeNowcast:
		return "Nowcast"
	case ModeForecast:
		return "Forecast"
	case ModeUserSupplied:
		return "UserSupplied"
	case ModeFast:
		return "Fast"
	default:
		return "Mode(unknown)"
	}
}
