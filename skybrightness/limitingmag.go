package skybrightness

import "math"

// LimitingMagModel converts a sky surface brightness (plus airmass) into a
// limiting magnitude. The conversion is explicit and named: a sky surface
// brightness is NOT the same quantity as a limiting magnitude.
type LimitingMagModel interface {
	// LimitingMagnitude returns the faintest detectable magnitude given the sky
	// surface brightness toward the pointing and the airmass along the path.
	LimitingMagnitude(sky SurfaceBrightnessV, airmass float64) (float64, error)
}

// Coefficients of the Schaefer (1990) naked-eye-limiting-magnitude relation as
// popularized by the Unihedron SQM↔NELM converter:
//
//	NELM = 7.93 − 5·log₁₀(10^(4.316 − m_sky/5) + 1)
//
// Reference: Schaefer 1990, PASP 102, 212; Unihedron SQM↔NELM converter. This
// is an empirical visual relation, not a detector S/N model.
const (
	nelmBright = 7.93  // bright-limit NELM as the sky becomes infinitely dark
	nelmScale  = 4.316 // sky-brightness scale constant
)

// defaultLimMagExtinction is the default V-band extinction coefficient
// (mag/airmass) used for the airmass penalty (KS 1991 Mauna Kea value).
const defaultLimMagExtinction = 0.172

// VisualLimitingMag is a naked-eye/visual [LimitingMagModel] using the
// Schaefer (1990) sky-brightness → NELM relation, with an optional extinction
// penalty k·(X−1) for additional dimming at airmass X > 1.
type VisualLimitingMag struct {
	k float64
}

// VisualLimitingMagOption configures a VisualLimitingMag.
type VisualLimitingMagOption func(*VisualLimitingMag)

// WithLimMagExtinction sets the V-band extinction coefficient (mag/airmass) used
// for the airmass penalty. The default is 0.172.
func WithLimMagExtinction(k float64) VisualLimitingMagOption {
	return func(c *VisualLimitingMag) { c.k = k }
}

// NewVisualLimitingMag creates a visual limiting-magnitude model.
func NewVisualLimitingMag(opts ...VisualLimitingMagOption) VisualLimitingMag {
	c := VisualLimitingMag{k: defaultLimMagExtinction}
	for _, opt := range opts {
		opt(&c)
	}

	return c
}

// LimitingMagnitude returns the visual limiting magnitude for the given sky
// surface brightness and airmass. An infinitely dark sky yields the bright
// limit (7.93); brighter skies and larger airmass reduce the limit.
func (c VisualLimitingMag) LimitingMagnitude(sky SurfaceBrightnessV, airmass float64) (float64, error) {
	// 10^(4.316 − m_sky/5): for an infinitely faint sky (m_sky = +Inf) this is
	// 0, so NELM → 7.93 with no special-casing required.
	nelm := nelmBright - 5*math.Log10(math.Pow(10, nelmScale-float64(sky)/5)+1)

	x := airmass
	if x < 1 {
		x = 1
	}

	return nelm - c.k*(x-1), nil
}
