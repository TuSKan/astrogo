package atmosphere

import "time"

// DatasetVersion identifies a specific, citable version of a dataset or
// algorithm implementation — a semver string, a DOI-versioned release tag,
// or similar.
type DatasetVersion string

// Fidelity classifies how a SourceRef's data relates to physical reality.
type Fidelity uint8

// The four fidelity levels.
const (
	// FidelityMeasured means the data is a direct field/satellite
	// measurement.
	FidelityMeasured Fidelity = iota

	// FidelityModelPropagated means the data is the output of a
	// radiative-transfer or other physical model, not itself a
	// measurement — e.g. the World Atlas 2015, which can never serve as
	// Level-3 observational validation (see docs/skybrightness.md §13).
	FidelityModelPropagated

	// FidelityPrior means the data is a prior/regional-average assumption
	// (e.g. a spectral-mixture regional prior) rather than sourced from
	// this specific site.
	FidelityPrior

	// FidelitySynthetic means the data is synthetic/test fixture data,
	// never to be presented as physically meaningful.
	FidelitySynthetic
)

// String implements fmt.Stringer.
func (f Fidelity) String() string {
	switch f {
	case FidelityMeasured:
		return "Measured"
	case FidelityModelPropagated:
		return "ModelPropagated"
	case FidelityPrior:
		return "Prior"
	case FidelitySynthetic:
		return "Synthetic"
	default:
		return "Fidelity(unknown)"
	}
}

// TimeRange is a closed time interval, [Start, End].
type TimeRange struct {
	Start, End time.Time
}

// SourceRef records the provenance of one dataset that contributed to a
// result: what it is, which version, over what period it was acquired,
// when this process retrieved it, its checksum, its licence, and its
// Fidelity. Every dataset that enters a pipeline attaches one of these at
// the point it is opened. Originally introduced for Sky Brightness V2
// (docs/skybrightness.md §6), lives here because it is a general
// data-provenance primitive, not specific to sky brightness — any
// atmospheric or dataset-backed package can use it.
type SourceRef struct {
	Name      string
	Version   DatasetVersion
	Acquired  TimeRange // the observation period the data represents
	Retrieved time.Time
	Checksum  string // "sha256:..."
	Licence   string
	Endpoint  string // remote.EndpointID as a string; empty for user-supplied data
	Fidelity  Fidelity
}
