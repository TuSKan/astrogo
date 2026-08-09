package skybrightness

import (
	"context"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
)

// ComponentID names one additive term of L_total. See
// docs/skybrightness.md §2 for the full decomposition.
type ComponentID uint8

// The eight components, in declaration order.
const (
	Airglow ComponentID = iota
	Zodiacal
	IntegratedStarlight
	DiffuseGalactic
	MoonScattered
	Twilight
	Artificial
	Aurora
)

const numComponents = int(Aurora) + 1

// String implements fmt.Stringer.
func (c ComponentID) String() string {
	switch c {
	case Airglow:
		return "Airglow"
	case Zodiacal:
		return "Zodiacal"
	case IntegratedStarlight:
		return "IntegratedStarlight"
	case DiffuseGalactic:
		return "DiffuseGalactic"
	case MoonScattered:
		return "MoonScattered"
	case Twilight:
		return "Twilight"
	case Artificial:
		return "Artificial"
	case Aurora:
		return "Aurora"
	default:
		return "ComponentID(unknown)"
	}
}

// IsNatural reports whether c is one of the seven natural-sky components
// (everything except Artificial).
func (c ComponentID) IsNatural() bool { return c != Artificial }

// ComponentMask is a bitset over ComponentID, used to select which
// components an evaluation includes.
type ComponentMask uint32

// Mask builds a ComponentMask from a list of ComponentIDs.
func Mask(ids ...ComponentID) ComponentMask {
	var m ComponentMask
	for _, id := range ids {
		m = m.Add(id)
	}

	return m
}

// Has reports whether c is set in m.
func (m ComponentMask) Has(c ComponentID) bool { return m&(1<<uint(c)) != 0 }

// Add returns m with c set.
func (m ComponentMask) Add(c ComponentID) ComponentMask { return m | 1<<uint(c) }

// AllComponents selects every ComponentID.
var AllComponents = ComponentMask(1<<uint(numComponents) - 1)

// NaturalOnly selects every ComponentID except Artificial.
var NaturalOnly = AllComponents &^ Mask(Artificial)

// AnthropogenicOnly selects only Artificial.
var AnthropogenicOnly = Mask(Artificial)

// EvalInput is the input a Component.Eval call receives.
type EvalInput struct {
	// Astro is the one coord.Context for this epoch. Hot paths must reuse
	// it, never build one per call — the repo-wide convention
	// coord.Context itself documents.
	Astro *coord.Context

	Directions []coord.AltAz
	Grid       SpectralGrid

	// Atmosphere is never nil: ClimatologyDefaultAtmosphere is substituted
	// when a Request specifies none.
	Atmosphere *atmosphere.Atmosphere

	Mode    Mode
	Options EvaluationOptions

	// Scratch is per-goroutine, caller-owned. A Component must not retain
	// it past the Eval call.
	Scratch *Scratch
}

// ComponentUncertainty is one Component's self-reported uncertainty
// contribution.
type ComponentUncertainty struct {
	// RelSigma is the fractional (relative) 1-sigma uncertainty on this
	// component's radiance, applied uniformly across the spectral grid
	// unless Spectral is set.
	RelSigma float64

	Group CovarianceGroup
	Kind  UncertaintyKind

	// Spectral optionally overrides RelSigma with a per-wavelength
	// relative sigma; len(Spectral) must equal the request grid's Len()
	// when non-nil.
	Spectral []float64
}

// ComponentReport is returned alongside a Component's SpectralField output.
type ComponentReport struct {
	Assumptions []string
	Uncertainty ComponentUncertainty
	Provenance  ComponentProvenance
	Quality     QualityFlags
}

// Component computes one additive term of L_total, in linear spectral
// radiance space (docs/skybrightness.md §2).
//
// Implementations must:
//   - be safe for concurrent use after construction;
//   - never retain any slice passed to Eval past the call;
//   - never read out — the engine pre-zeroes it before calling Eval, so a
//     Component only ever writes.
type Component interface {
	ID() ComponentID
	Algorithm() AlgorithmRef
	Eval(ctx context.Context, in EvalInput, out SpectralField) (ComponentReport, error)
}
