package skybrightness

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
)

// DatasetVersion identifies a specific, citable version of a dataset or
// algorithm implementation — a semver string, a DOI-versioned release tag,
// or similar.
type DatasetVersion string

// Fidelity classifies how a SourceRef's data relates to physical reality.
type Fidelity uint8

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
// Result: what it is, which version, over what period it was acquired,
// when this process retrieved it, its checksum, its licence, and its
// Fidelity. Every dataset that enters the pipeline attaches one of these
// at the point it is opened (docs/skybrightness.md §6).
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

// FallbackRecord documents one explicit mode fallback that occurred while
// producing a Result (see EvaluationOptions.Fallback). Fallback defaults to
// forbidden; a record here only ever exists when the caller opted in.
type FallbackRecord struct {
	From, To Mode
	Reason   string
	At       time.Time
}

// AlgorithmRef names one piece of physics or one dataset-processing
// algorithm precisely enough to reproduce or audit it: an implementation
// name, its semantic version, and (where applicable) the paper it
// implements.
type AlgorithmRef struct {
	Name     string // e.g. "natural.LegacyMoonlight"
	Version  string // semver of the implementation
	Citation string // e.g. "Krisciunas & Schaefer (1991), PASP 103, 1033"
}

// ComponentProvenance is the provenance contributed by one Component's
// evaluation.
type ComponentProvenance struct {
	Component ComponentID
	Algorithm AlgorithmRef
	Datasets  []SourceRef
}

// AtmosphereProvenance records where an AtmosphereState came from and how
// current it is.
type AtmosphereProvenance struct {
	Source   SourceRef
	IssueAt  time.Time // when the state was issued (nowcast/forecast)
	LeadTime time.Duration
}

// SurrogateRef is a placeholder for Phase 6 surrogate provenance; nil
// until that phase populates it.
type SurrogateRef struct{ Version DatasetVersion }

// CalibrationRef is a placeholder for Phase 7 calibration provenance; nil
// until that phase populates it.
type CalibrationRef struct{ Version DatasetVersion }

// PassbandRef records a Passband's identity and dataset version for
// provenance purposes.
type PassbandRef struct {
	ID      PassbandID
	Version DatasetVersion
}

// Provenance is attached to every Result. Two evaluations with identical
// inputs, models, and datasets produce identical Provenance, and
// identical Digest(), within documented floating-point tolerances.
type Provenance struct {
	SchemaVersion string
	Engine        AlgorithmRef
	Mode          Mode
	Components    []ComponentProvenance
	Transmission  AlgorithmRef
	Datasets      []SourceRef
	Atmosphere    AtmosphereProvenance
	Surrogate     *SurrogateRef
	Calibration   *CalibrationRef
	Passbands     []PassbandRef
	Fallbacks     []FallbackRecord
	Seed          uint64
	GridID        GridID
	EvaluatedAt   time.Time
}

// MarshalJSON implements json.Marshaler with a deterministic serialization
// shape: json.Marshal on a struct already emits fields in declaration
// order (stable), and every field here is either a primitive, a slice of
// primitives/structs, or a RFC3339Nano timestamp — no maps, so no key-order
// ambiguity exists to begin with.
func (p Provenance) MarshalJSON() ([]byte, error) {
	type alias Provenance // avoid infinite recursion into this method

	b, err := json.Marshal(alias(p))
	if err != nil {
		return nil, fmt.Errorf("skybrightness: marshal Provenance: %w", err)
	}

	return b, nil
}

// Digest returns the SHA-256 of Provenance's canonical JSON serialization.
// Identical inputs, models, and datasets produce an identical digest.
func (p Provenance) Digest() [32]byte {
	b, err := p.MarshalJSON()
	if err != nil {
		return [32]byte{} // MarshalJSON above cannot fail for this shape; defensive only
	}

	return sha256.Sum256(b)
}
