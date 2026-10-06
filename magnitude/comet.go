package magnitude

import (
	"math"

	"github.com/TuSKan/astrogo/angle"
)

// CometApparent computes the total apparent magnitude of a comet, as JPL
// defines it for Horizons' T-mag:
//
//	m_total = M₁ + 5·log₁₀(Δ) + k₁·log₁₀(r)
//
// It has no phase term, unlike the nuclear magnitude.
//
// Parameters:
//   - M1: absolute total magnitude (SBDB's M1)
//   - k1: total magnitude slope in log r (SBDB's K1; typically ~10 for active
//     comets, ~5 for inactive)
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

// CometNuclearApparent computes the nuclear apparent magnitude of a comet,
// as JPL defines it for Horizons' N-mag:
//
//	m_nuclear = M₂ + 5·log₁₀(Δ) + k₂·log₁₀(r) + PC·β
//
// Parameters:
//   - M2: absolute nuclear magnitude (SBDB's M2)
//   - k2: nuclear magnitude slope in log r (SBDB's K2; ~5 for pure
//     inverse-square)
//   - pc: nuclear phase coefficient in magnitudes per degree (SBDB's PC);
//     zero where SBDB publishes none, as Horizons then applies none
//   - r: heliocentric distance (AU)
//   - delta: geocentric distance (AU)
//   - phase: the Sun–target–observer angle β
//
// Until #548 the phase term was missing, which made a nucleus brighter by
// PC·β: 0.21 mag for 13P/Olbers at 7°, against Horizons' own N-mag.
func CometNuclearApparent(M2, k2, pc, r, delta float64, phase angle.Angle) float64 {
	if r <= 0 || delta <= 0 {
		return M2
	}

	return M2 + 5*math.Log10(delta) + k2*math.Log10(r) + pc*phase.Degrees()
}
