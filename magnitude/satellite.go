package magnitude

import (
	"math"

	"github.com/TuSKan/astrogo/angle"
)

// ── Satellite Magnitude ──────────────────────────────────────────────────────

// SatPhaseModel identifies the phase function for satellite brightness.
type SatPhaseModel int

const (
	// PhaseSphere uses a Lambertian sphere model: Ψ(α) = (1 + cos α) / 2
	PhaseSphere SatPhaseModel = iota

	// PhaseCylinder uses a diffuse cylinder model (e.g. rocket bodies):
	// Ψ(α) = (sin α + (π−α)·cos α) / π
	PhaseCylinder
)

// StdMagConvention identifies the satellite standard magnitude convention.
type StdMagConvention int

const (
	// ConventionMcCants uses the McCants/Quicksat convention:
	// standard magnitude at 1000 km range, 90° phase, maximum brightness,
	// 100% illumination assumption.
	ConventionMcCants StdMagConvention = iota

	// ConventionMolczan uses the Molczan convention:
	// same range/phase reference but mean brightness, 50% illumination.
	// Intrinsically ~1.4 mag fainter than McCants for the same satellite.
	ConventionMolczan
)

// SatelliteApparent computes the apparent visual magnitude of an artificial satellite.
//
//	m_obs = m_std − 15.75 + 2.5·log₁₀(range²) − 2.5·log₁₀(Ψ(α))
//
// equivalent to:
//
//	m_obs = m_std + 5·log₁₀(range_km / 1000) − 2.5·log₁₀(Ψ(α))
//
// Parameters:
//   - stdMag: standard magnitude from catalog (McCants or Molczan convention)
//   - conv: which convention stdMag uses (affects phase reference interpretation)
//   - rangeKm: observer–satellite range in kilometres
//   - alpha: phase angle (Sun–satellite–observer)
//   - shape: phase function model (sphere or cylinder)
func SatelliteApparent(stdMag float64, conv StdMagConvention, rangeKm float64, alpha angle.Angle, shape SatPhaseModel) float64 {
	if rangeKm <= 0 {
		return stdMag
	}

	// Distance modulus relative to 1000 km reference.
	distMod := 5 * math.Log10(rangeKm/1000)

	// Phase function.
	psi := satPhaseFunction(alpha.Radians(), shape)
	if psi <= 0 {
		psi = 1e-30
	}

	phaseMag := -2.5 * math.Log10(psi)

	// McCants convention already includes the phase at 90° in the reference.
	// Molczan is similar but with different assumptions.
	// The phase correction is relative to the reference geometry (α=90°).
	refPsi := satPhaseFunction(math.Pi/2, shape) // Ψ at 90°
	refMag := -2.5 * math.Log10(refPsi)

	return stdMag + distMod + phaseMag - refMag
}

// satPhaseFunction evaluates the phase function for the given shape model.
func satPhaseFunction(alphaRad float64, shape SatPhaseModel) float64 {
	switch shape {
	case PhaseCylinder:
		sinA := math.Sin(alphaRad)
		cosA := math.Cos(alphaRad)

		return (sinA + (math.Pi-alphaRad)*cosA) / math.Pi
	default: // PhaseSphere
		return (1 + math.Cos(alphaRad)) / 2
	}
}
