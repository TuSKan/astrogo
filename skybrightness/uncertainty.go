package skybrightness

import "errors"

// CovarianceGroup names one of the nine sources of uncertainty this engine
// tracks separately. Contributions within one group are combined linearly
// (treated as fully correlated); contributions across groups are combined
// in quadrature. See docs/skybrightness.md §10 for why naive
// all-quadrature combination is wrong (VIIRS intensity error, for one, is
// spatially correlated across an entire city, not an independent per-pixel
// draw).
type CovarianceGroup uint8

// The nine covariance groups, in declaration order.
const (
	GroupEmissionIntensity CovarianceGroup = iota
	GroupSourceSpectrum
	GroupAngularEmission
	GroupAerosol
	GroupCloud
	GroupNatural
	GroupSurrogate
	GroupCalibration
	GroupInputAge
)

const numCovarianceGroups = int(GroupInputAge) + 1

// String implements fmt.Stringer.
func (g CovarianceGroup) String() string {
	switch g {
	case GroupEmissionIntensity:
		return "EmissionIntensity"
	case GroupSourceSpectrum:
		return "SourceSpectrum"
	case GroupAngularEmission:
		return "AngularEmission"
	case GroupAerosol:
		return "Aerosol"
	case GroupCloud:
		return "Cloud"
	case GroupNatural:
		return "Natural"
	case GroupSurrogate:
		return "Surrogate"
	case GroupCalibration:
		return "Calibration"
	case GroupInputAge:
		return "InputAge"
	default:
		return "CovarianceGroup(unknown)"
	}
}

// UncertaintyKind distinguishes the three kinds of uncertainty the mandate
// requires this engine to distinguish, rather than lumping them together.
type UncertaintyKind uint8

// The three kinds of uncertainty.
const (
	Aleatoric UncertaintyKind = iota
	Epistemic
	Measurement
)

// String implements fmt.Stringer.
func (k UncertaintyKind) String() string {
	switch k {
	case Aleatoric:
		return "Aleatoric"
	case Epistemic:
		return "Epistemic"
	case Measurement:
		return "Measurement"
	default:
		return "UncertaintyKind(unknown)"
	}
}

// UncertaintyMode selects how uncertainty is propagated through an
// evaluation.
type UncertaintyMode uint8

const (
	// UncNone disables uncertainty propagation entirely.
	UncNone UncertaintyMode = iota

	// UncLinearized uses first-order linearized propagation with
	// covariance groups — the only mode Phase 1 implements.
	UncLinearized

	// UncEnsemble uses deterministic ensemble members. Not implemented
	// before Phase 6/7.
	UncEnsemble

	// UncMonteCarlo uses seeded Monte Carlo sampling. Not implemented
	// before Phase 6/7.
	UncMonteCarlo
)

// String implements fmt.Stringer.
func (m UncertaintyMode) String() string {
	switch m {
	case UncNone:
		return "None"
	case UncLinearized:
		return "Linearized"
	case UncEnsemble:
		return "Ensemble"
	case UncMonteCarlo:
		return "MonteCarlo"
	default:
		return "UncertaintyMode(unknown)"
	}
}

// ErrUncertaintyModeUnimplemented is returned when EvaluationOptions.Uncertainty
// requests UncEnsemble or UncMonteCarlo — neither is implemented before
// Phase 6/7 (docs/skybrightness.md §14). Returning this error, rather than
// silently degrading to UncLinearized or UncNone, is the honest choice: a
// caller that asked for ensemble/Monte Carlo uncertainty and silently got
// something else would draw wrong conclusions from UncertaintyResult.
var ErrUncertaintyModeUnimplemented = errors.New("skybrightness: uncertainty mode not implemented before Phase 6/7")

// GroupContribution is one covariance group's contribution to the total
// uncertainty of a Result.
type GroupContribution struct {
	Group    CovarianceGroup
	Kind     UncertaintyKind
	Variance float64 // (relative sigma)^2, this group's contribution
	Share    float64 // this group's fraction of total variance, in [0,1]
}

// DomainWarning documents a request that fell outside a model's validated
// or trained domain (e.g. QualityFlagOutOfSurrogateDomain).
type DomainWarning struct {
	Component ComponentID
	Message   string
}

// UncertaintyResult carries every uncertainty output the mandate
// requires: per-wavelength percentile fields, a per-wavelength relative
// sigma, per-group and per-component variance shares, and any domain
// warnings raised while producing the Result.
type UncertaintyResult struct {
	Mode UncertaintyMode

	P05, P50, P95 SpectralField
	RelSigma      SpectralField // relative (fractional) 1-sigma, per wavelength

	ByGroup     [numCovarianceGroups]GroupContribution
	ByComponent [numComponents]float64 // this component's fraction of total variance

	Warnings []DomainWarning
}
