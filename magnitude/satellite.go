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
	// standard magnitude at 1000 km range, full phase (100% illumination),
	// representing the brightest-likely ("maximum") brightness.
	ConventionMcCants StdMagConvention = iota

	// ConventionMolczan uses the Molczan convention:
	// standard magnitude at 1000 km range, 90° phase (50% illumination),
	// representing mean brightness. Intrinsically ~1.4 mag fainter than
	// McCants for the same satellite (see molczanOffset).
	ConventionMolczan
)

// molczanOffset is the total magnitude difference between the Molczan
// (mcnames) and McCants (quicksat) standard-magnitude conventions, ~1.4 mag,
// per https://www.mmccants.org/tles/intrmagdef.html. It is the sum of two
// independent ~0.7 mag effects:
//   - illumination/phase convention (Molczan 50% vs McCants full phase):
//     2.5·log₁₀(2) ≈ 0.75 mag
//   - mean vs. maximum ("brightest likely") brightness: ≈0.7 mag, a
//     definitional (non-geometric) difference
const molczanOffset = 1.45 // 2.5·log₁₀(2) + 0.7

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

	// Normalize Molczan standard magnitudes to the McCants reference frame.
	// A Molczan value is ~1.4 mag fainter than the McCants value for the same
	// object (see molczanOffset), so subtract the offset to convert.
	m := stdMag
	if conv == ConventionMolczan {
		m -= molczanOffset
	}

	// Distance modulus relative to 1000 km reference.
	distMod := 5 * math.Log10(rangeKm/1000)

	// Phase function.
	psi := satPhaseFunction(alpha.Radians(), shape)
	if psi <= 0 {
		psi = 1e-30
	}

	phaseMag := -2.5 * math.Log10(psi)

	// The phase correction is relative to the reference geometry (α=90°).
	refPsi := satPhaseFunction(math.Pi/2, shape) // Ψ at 90°
	refMag := -2.5 * math.Log10(refPsi)

	return m + distMod + phaseMag - refMag
}

// satPhaseFunction evaluates the phase function for the given shape model.
func satPhaseFunction(alphaRad float64, shape SatPhaseModel) float64 {
	switch shape { //nolint:exhaustive // PhaseSphere handled by default
	case PhaseCylinder:
		sinA := math.Sin(alphaRad)
		cosA := math.Cos(alphaRad)

		return (sinA + (math.Pi-alphaRad)*cosA) / math.Pi
	default: // PhaseSphere
		return (1 + math.Cos(alphaRad)) / 2
	}
}
