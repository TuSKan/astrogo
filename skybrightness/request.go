package skybrightness

import (
	"errors"
	"fmt"
	"time"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
)

// ErrInvalidRequest is returned by Request.Validate.
var ErrInvalidRequest = errors.New("skybrightness: invalid request")

// ComponentSelection controls which components an evaluation includes and
// whether their individual SpectralFields are materialized.
type ComponentSelection struct {
	// Include selects components to evaluate. The zero value means
	// AllComponents.
	Include ComponentMask
	// Exclude removes components from Include.
	Exclude ComponentMask
	// Materialize controls memory: when false, components are summed
	// straight into Result.Total and Result.Components carries only their
	// ComponentReport, never a SpectralField (proven bit-identical to
	// Materialize: true for Total — see the Phase 1 invariant tests).
	Materialize bool
}

func (s ComponentSelection) mask() ComponentMask {
	include := s.Include
	if include == 0 {
		include = AllComponents
	}

	return include &^ s.Exclude
}

// DerivedMask selects which DerivedQuantities an evaluation computes.
// Nothing in DerivedQuantities is computed unless requested — a caller
// that only wants Result.Total pays nothing extra for it.
type DerivedMask uint32

// The seven derivable quantities.
const (
	DerivePassbands DerivedMask = 1 << iota
	DeriveLuminance
	DeriveIrradiance
	DeriveAllSkyStats
	DeriveAnthroRatio
	DeriveLimitingMag
	DeriveDetectorBackground
)

// Has reports whether every bit in want is set in m.
func (m DerivedMask) Has(want DerivedMask) bool { return m&want == want }

// FallbackPolicy selects whether and how a Mode may fall back to another
// mode when its required input is unavailable. The default,
// FallbackForbidden, means a missing input is an error — never a silent
// substitution (docs/skybrightness.md §7).
type FallbackPolicy uint8

// The three fallback policies.
const (
	FallbackForbidden FallbackPolicy = iota
	FallbackToClimatology
	FallbackToFast
)

// String implements fmt.Stringer.
func (p FallbackPolicy) String() string {
	switch p {
	case FallbackForbidden:
		return "Forbidden"
	case FallbackToClimatology:
		return "ToClimatology"
	case FallbackToFast:
		return "ToFast"
	default:
		return "FallbackPolicy(unknown)"
	}
}

// BufferPool lets a caller reuse SpectralField/Scratch allocations across
// repeated Evaluate calls — the plan package's scoring loop is the
// motivating use case. The zero value (nil) means no pooling.
type BufferPool struct {
	scratch *Scratch
}

// EvaluationOptions configures one Evaluate/EvaluateBatch call beyond the
// physical Request itself.
type EvaluationOptions struct {
	ComputeTransmission bool
	Derived             DerivedMask
	Uncertainty         UncertaintyMode
	UncertaintySamples  int
	Seed                uint64
	Fallback            FallbackPolicy
	MaxInputAge         time.Duration // 0 = no staleness check
	Parallelism         int           // 0 = runtime.GOMAXPROCS
	ScatteringOrders    int           // 0 = engine default; recorded in Provenance
	Buffers             *BufferPool   // optional caller-owned reuse
	LimitingMag         LimitingMagModel
	Instrument          *Instrument
}

// Request is one spectral sky-radiance evaluation.
type Request struct {
	// Astro is the one coord.Context for this epoch. Build it once and
	// reuse it — the repo-wide hot-path convention.
	Astro      *coord.Context
	Directions []coord.AltAz
	Grid       SpectralGrid
	Passbands  []*Passband
	Mode       Mode
	// Atmosphere may be nil; ClimatologyDefaultAtmosphere(nil) is
	// substituted, which is itself recorded in Provenance.Fallbacks only
	// if Mode != ModeClimatology (a Climatology request supplying no
	// atmosphere is using the mode as intended, not falling back).
	Atmosphere *atmosphere.State
	Selection  ComponentSelection
	Options    EvaluationOptions
}

// Sentinel components of the error Request.Validate returns, wrapped with
// %w so a caller can errors.Is against the specific violation.
var (
	errRequestNilAstro     = errors.New("Request.Astro must not be nil")
	errRequestNoDirections = errors.New("Request.Directions must not be empty")
	errRequestZeroGrid     = errors.New("Request.Grid must not be the zero value")
)

// Validate checks the request's internal consistency.
func (r Request) Validate() error {
	if r.Astro == nil {
		return fmt.Errorf("%w: %w", ErrInvalidRequest, errRequestNilAstro)
	}

	if len(r.Directions) == 0 {
		return fmt.Errorf("%w: %w", ErrInvalidRequest, errRequestNoDirections)
	}

	if r.Grid.Len() == 0 {
		return fmt.Errorf("%w: %w", ErrInvalidRequest, errRequestZeroGrid)
	}

	return nil
}

// BatchRequest evaluates the same set of directions across multiple
// epochs.
type BatchRequest struct {
	Astro      []*coord.Context // one per epoch
	Directions []coord.AltAz    // shared across epochs
	Grid       SpectralGrid
	Passbands  []*Passband
	Mode       Mode
	// Atmosphere has length 1 (shared across every epoch) or
	// len(Astro) (one per epoch).
	Atmosphere []*atmosphere.State
	Selection  ComponentSelection
	Options    EvaluationOptions
}

func (r BatchRequest) at(i int) Request {
	var atm *atmosphere.State

	switch {
	case len(r.Atmosphere) == 1:
		atm = r.Atmosphere[0]
	case len(r.Atmosphere) > i:
		atm = r.Atmosphere[i]
	}

	return Request{
		Astro: r.Astro[i], Directions: r.Directions, Grid: r.Grid,
		Passbands: r.Passbands, Mode: r.Mode, Atmosphere: atm,
		Selection: r.Selection, Options: r.Options,
	}
}
