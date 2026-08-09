package skybrightness

import "math"

// LimitingMagInput is the input to LimitingMagModel.LimitingMagnitude — a
// sky background expressed in both magnitude systems (a model uses
// whichever it needs), the passband it was computed in, and the airmass
// along the line of sight.
type LimitingMagInput struct {
	Passband PassbandID
	SkyVega  SurfaceBrightnessVega
	SkyAB    SurfaceBrightnessAB
	Airmass  float64
}

// LimitingMagModel converts a sky background (plus airmass) into a
// limiting magnitude. The conversion is explicit and named — a sky
// surface brightness is never treated as interchangeable with a limiting
// magnitude (docs/skybrightness.md §1).
type LimitingMagModel interface {
	Algorithm() AlgorithmRef
	LimitingMagnitude(in LimitingMagInput) (float64, error)
}

// Coefficients of the Schaefer (1990) naked-eye-limiting-magnitude
// relation as popularized by the Unihedron SQM<->NELM converter:
//
//	NELM = 7.93 - 5*log10(10^(4.316 - m_sky/5) + 1)
//
// Reference: Schaefer 1990, PASP 102, 212; Unihedron SQM<->NELM converter.
// This is an empirical visual relation, not a detector S/N model. Carried
// forward verbatim from astrogo v1 (docs/skybrightness.md §15) — same
// constants, same formula.
const (
	schaeferNelmBright = 7.93  // bright-limit NELM as the sky becomes infinitely dark
	schaeferNelmScale  = 4.316 // sky-brightness scale constant
)

// DefaultSchaeferNELMExtinction is the default V-band extinction
// coefficient (mag/airmass) for SchaeferNELM's airmass penalty
// (Krisciunas & Schaefer 1991 Mauna Kea value).
const DefaultSchaeferNELMExtinction = 0.172

// SchaeferNELM is a naked-eye/visual LimitingMagModel using the Schaefer
// (1990) sky-brightness -> NELM relation, with an optional extinction
// penalty k*(X-1) for additional dimming at airmass X > 1. It operates on
// LimitingMagInput.SkyVega, which the caller is responsible for having
// computed in an approximately-Johnson-V passband — the formula's
// constants were calibrated against V-band visual observations and are
// not meaningful in an arbitrary passband.
type SchaeferNELM struct {
	k float64
}

// SchaeferNELMOption configures a SchaeferNELM.
type SchaeferNELMOption func(*SchaeferNELM)

// WithExtinction sets the V-band extinction coefficient (mag/airmass)
// used for the airmass penalty. The default is
// DefaultSchaeferNELMExtinction.
func WithExtinction(k float64) SchaeferNELMOption {
	return func(c *SchaeferNELM) { c.k = k }
}

// NewSchaeferNELM creates a Schaefer (1990) visual limiting-magnitude
// model.
func NewSchaeferNELM(opts ...SchaeferNELMOption) *SchaeferNELM {
	c := &SchaeferNELM{k: DefaultSchaeferNELMExtinction}
	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Algorithm implements LimitingMagModel.
func (c *SchaeferNELM) Algorithm() AlgorithmRef {
	return AlgorithmRef{
		Name:     "skybrightness.SchaeferNELM",
		Version:  "1.0.0",
		Citation: "Schaefer (1990), PASP 102, 212; Unihedron SQM<->NELM converter",
	}
}

// LimitingMagnitude returns the visual limiting magnitude for the given
// sky background and airmass. An infinitely dark sky yields the bright
// limit (7.93); brighter skies and larger airmass reduce the limit.
func (c *SchaeferNELM) LimitingMagnitude(in LimitingMagInput) (float64, error) {
	// 10^(4.316 - m_sky/5): for an infinitely faint sky (m_sky = +Inf)
	// this is 0, so NELM -> 7.93 with no special-casing required.
	nelm := schaeferNelmBright - 5*math.Log10(math.Pow(10, schaeferNelmScale-float64(in.SkyVega)/5)+1)

	x := in.Airmass
	if x < 1 {
		x = 1
	}

	return nelm - c.k*(x-1), nil
}
