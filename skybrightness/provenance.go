package skybrightness

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/TuSKan/astrogo/atmosphere"
)

// DatasetVersion is a general data-provenance primitive that lives in
// package atmosphere (a peer scientific-engine package, not specific to
// sky brightness) since any atmospheric or dataset-backed consumer needs
// it, not just this package — along with Fidelity, TimeRange, and
// SourceRef below. Aliased here, not redeclared, so skybrightness's own
// Provenance/ComponentProvenance/Passband can keep writing the short
// names.
//
//nolint:revive // one doc comment intentionally describes this whole small alias block, not any single member's name
type (
	DatasetVersion = atmosphere.DatasetVersion
	Fidelity       = atmosphere.Fidelity
	TimeRange      = atmosphere.TimeRange
	SourceRef      = atmosphere.SourceRef
)

// The four fidelity levels, aliased from package atmosphere — see
// atmosphere.Fidelity's doc comment for what each means.
const (
	FidelityMeasured        = atmosphere.FidelityMeasured
	FidelityModelPropagated = atmosphere.FidelityModelPropagated
	FidelityPrior           = atmosphere.FidelityPrior
	FidelitySynthetic       = atmosphere.FidelitySynthetic
)

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
	Name     string // e.g. "natural.VBandMoonlight"
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

// AtmosphereProvenance records where an atmosphere.Atmosphere came from and
// how current it is — an alias for atmosphere.Provenance, the type
// atmosphere.Atmosphere.Provenance() itself returns.
type AtmosphereProvenance = atmosphere.Provenance

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

// String implements fmt.Stringer with a human-readable multi-line summary
// — engine, mode, components, datasets, fallbacks, and the digest — so a
// caller can print(res.Provenance) directly without knowing this type
// also implements json.Marshaler. A caller wanting the full JSON should
// call json.Marshal(res.Provenance) (or MarshalJSON) explicitly; this
// method is for eyeballing, not for storage or reproducible hashing.
func (p Provenance) String() string {
	var b strings.Builder

	fmt.Fprintf(&b, "Provenance (schema %s)\n", p.SchemaVersion)
	fmt.Fprintf(&b, "  Engine:       %s %s", p.Engine.Name, p.Engine.Version)

	if p.Engine.Citation != "" {
		fmt.Fprintf(&b, " (%s)", p.Engine.Citation)
	}

	fmt.Fprintf(&b, "\n  Mode:         %s\n", p.Mode)

	if p.Transmission.Name != "" {
		fmt.Fprintf(&b, "  Transmission: %s %s\n", p.Transmission.Name, p.Transmission.Version)
	}

	for _, c := range p.Components {
		fmt.Fprintf(&b, "  Component:    %-16s %s %s\n", c.Component, c.Algorithm.Name, c.Algorithm.Version)
	}

	for _, d := range p.Datasets {
		fmt.Fprintf(&b, "  Dataset:      %s %s (%s)\n", d.Name, d.Version, d.Fidelity)
	}

	for _, f := range p.Fallbacks {
		fmt.Fprintf(&b, "  Fallback:     %s -> %s (%s)\n", f.From, f.To, f.Reason)
	}

	fmt.Fprintf(&b, "  Evaluated at: %s\n", p.EvaluatedAt.Format(time.RFC3339Nano))
	fmt.Fprintf(&b, "  Digest:       %x", p.Digest())

	return b.String()
}
