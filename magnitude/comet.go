package magnitude

import "math"

// CometApparent computes the total apparent magnitude of a comet using the
// IAU standard model:
//
//	m_total = M₁ + 5·log₁₀(Δ) + k₁·log₁₀(r)
//
// Parameters:
//   - M1: absolute total magnitude
//   - k1: activity parameter (typically ~10 for active comets, ~5 for inactive)
//   - r: heliocentric distance (AU)
//   - delta: geocentric distance (AU)
//
// Note: comet magnitudes are inherently unpredictable due to outbursts,
// fragmentation, and variable activity. Predictions are rarely better than
// ±1 mag regardless of model quality.
func CometApparent(M1, k1, r, delta float64) float64 {
	if r <= 0 || delta <= 0 {
		return M1
	}
	return M1 + 5*math.Log10(delta) + k1*math.Log10(r)
}

// CometNuclearApparent computes the nuclear apparent magnitude of a comet:
//
//	m_nuclear = M₂ + 5·log₁₀(Δ) + k₂·log₁₀(r)
//
// Parameters:
//   - M2: absolute nuclear magnitude
//   - k2: nuclear activity parameter (typically ~5 for pure inverse-square)
//   - r: heliocentric distance (AU)
//   - delta: geocentric distance (AU)
func CometNuclearApparent(M2, k2, r, delta float64) float64 {
	if r <= 0 || delta <= 0 {
		return M2
	}
	return M2 + 5*math.Log10(delta) + k2*math.Log10(r)
}
