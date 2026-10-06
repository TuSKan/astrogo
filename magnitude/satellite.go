package magnitude

import (
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/unit"
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
	// representing mean brightness. Numerically ~1.4 mag fainter than the
	// McCants value for the same satellite; SatelliteApparent's predictions
	// from it come out ~0.7 mag fainter, the mean-vs-brightest half of that.
	ConventionMolczan
)

// satelliteReferenceRange is the range a standard magnitude is quoted at:
// 1000 km, for both the McCants and Molczan conventions. The distance modulus
// is relative to it, so it is the one length in this formula that is a
// definition rather than an observation.
const satelliteReferenceRange unit.Length = 1_000_000

// SatelliteApparent computes the apparent visual magnitude of an artificial
// satellite from a standard magnitude, at the convention's own reference
// geometry:
//
//	m = m_std + 5·log₁₀(range / 1000 km) − 2.5·log₁₀(Ψ(α) / Ψ(α_ref))
//
// with α_ref the convention's reference phase angle: 0° (full phase) for
// McCants/Quicksat, 90° for Molczan, as https://www.mmccants.org/tles/intrmagdef.html
// defines them. A standard magnitude therefore reproduces itself at 1000 km
// and its own reference phase.
//
// The conventions are not converted into one another. Of the ~1.4 mag between
// them, the phase-definition half is what α_ref accounts for; the other half
// is that McCants quotes the "brightest likely" orientation and Molczan an
// "average" one, and that page asks a program to keep it: a Molczan-based
// prediction should come out about 0.7 mag fainter than a McCants-based one
// for the same object, which this does.
//
// Until #560 the phase was measured from 90° whatever the convention — so a
// McCants magnitude came out 0.75 mag (sphere) or 1.24 mag (cylinder) too
// bright at its own reference geometry — and a Molczan magnitude was shifted
// by the whole 1.45 mag as well.
//
// Parameters:
//   - stdMag: standard magnitude from catalog (McCants or Molczan convention)
//   - conv: which convention stdMag uses, which fixes the reference phase
//   - observerRange: observer–satellite range
//   - alpha: phase angle (Sun–satellite–observer)
//   - shape: phase function model (sphere or cylinder)
func SatelliteApparent(
	stdMag float64, conv StdMagConvention, observerRange unit.Length,
	alpha angle.Angle, shape SatPhaseModel,
) float64 {
	if observerRange <= 0 {
		return stdMag
	}

	// Distance modulus relative to the 1000 km reference. A ratio of two
	// lengths is dimensionless, so it is taken on the raw values.
	distMod := 5 * math.Log10(float64(observerRange)/float64(satelliteReferenceRange))

	// Phase function.
	psi := satPhaseFunction(alpha.Radians(), shape)
	if psi <= 0 {
		psi = 1e-30
	}

	// The reference phase is the convention's: full phase for McCants, 90°
	// for Molczan.
	refAlpha := 0.0
	if conv == ConventionMolczan {
		refAlpha = math.Pi / 2
	}

	refPsi := satPhaseFunction(refAlpha, shape)

	return stdMag + distMod - 2.5*math.Log10(psi/refPsi)
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
