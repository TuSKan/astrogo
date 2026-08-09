package skybrightness

import (
	"time"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
)

// RequestBuilder builds a Request via chained, named methods instead of a
// hand-nested struct literal — mirroring atmosphere.NewBuilder()'s
// established pattern. Request{...} struct-literal construction stays
// fully valid (no field is hidden); RequestBuilder is the recommended,
// documented idiom for readability, not a mandatory gate. Point (see
// derived.go) builds its own Request through this same path rather than a
// second, parallel construction — one general construction mechanism,
// configured via chained options.
type RequestBuilder struct {
	req Request
}

// NewRequestBuilder starts a builder for the one required core of a
// Request: the shared coord.Context, the directions to evaluate, and the
// spectral grid. Everything else defaults to its zero value and is set via
// the chained methods below.
func NewRequestBuilder(astro *coord.Context, directions []coord.AltAz, grid SpectralGrid) *RequestBuilder {
	return &RequestBuilder{req: Request{Astro: astro, Directions: directions, Grid: grid}}
}

// Atmosphere sets the evaluation's atmospheric state.
func (b *RequestBuilder) Atmosphere(a *atmosphere.Atmosphere) *RequestBuilder {
	b.req.Atmosphere = a
	return b
}

// Mode sets the evaluation mode.
func (b *RequestBuilder) Mode(m Mode) *RequestBuilder {
	b.req.Mode = m
	return b
}

// Passbands sets the passbands to integrate derived surface-brightness
// quantities against.
func (b *RequestBuilder) Passbands(p ...*Passband) *RequestBuilder {
	b.req.Passbands = p
	return b
}

// Select sets which components are evaluated (include/exclude masks — the
// zero value for include means AllComponents).
func (b *RequestBuilder) Select(include, exclude ComponentMask) *RequestBuilder {
	b.req.Selection.Include = include
	b.req.Selection.Exclude = exclude

	return b
}

// Materialize requests that each component's own SpectralField be
// materialized (Result.Components), not just summed into Result.Total.
func (b *RequestBuilder) Materialize() *RequestBuilder {
	b.req.Selection.Materialize = true
	return b
}

// Transmission requests line-of-sight transmission be computed
// (Result.Transmission), when the engine has a TransmissionModel
// configured.
func (b *RequestBuilder) Transmission() *RequestBuilder {
	b.req.Options.ComputeTransmission = true
	return b
}

// Derive sets which DerivedQuantities to compute.
func (b *RequestBuilder) Derive(mask DerivedMask) *RequestBuilder {
	b.req.Options.Derived.Mask |= mask
	return b
}

// LimitingMag sets the model DeriveLimitingMag uses (also implicitly
// requests DeriveLimitingMag via Derive).
func (b *RequestBuilder) LimitingMag(m LimitingMagModel) *RequestBuilder {
	b.req.Options.Derived.Mask |= DeriveLimitingMag
	b.req.Options.Derived.LimitingMag = m

	return b
}

// Instrument sets the model DeriveDetectorBackground uses (also implicitly
// requests DeriveDetectorBackground via Derive).
func (b *RequestBuilder) Instrument(i *Instrument) *RequestBuilder {
	b.req.Options.Derived.Mask |= DeriveDetectorBackground
	b.req.Options.Derived.Instrument = i

	return b
}

// Uncertainty sets the uncertainty-propagation mode, sample count (ignored
// outside UncEnsemble/UncMonteCarlo), and RNG seed.
func (b *RequestBuilder) Uncertainty(mode UncertaintyMode, samples int, seed uint64) *RequestBuilder {
	b.req.Options.Uncertainty = UncertaintyOptions{Mode: mode, Samples: samples, Seed: seed}
	return b
}

// Fallback sets the evaluation's FallbackPolicy. The zero value,
// FallbackForbidden, is already the default — call this only to opt into
// FallbackToClimatology/FallbackToFast.
func (b *RequestBuilder) Fallback(p FallbackPolicy) *RequestBuilder {
	b.req.Options.Fallback = p
	return b
}

// MaxInputAge sets the maximum age a supplied atmosphere.Atmosphere may
// have before it is treated as stale.
func (b *RequestBuilder) MaxInputAge(d time.Duration) *RequestBuilder {
	b.req.Options.MaxInputAge = d
	return b
}

// Performance sets the throughput-related options (parallelism, scattering
// orders, buffer reuse) as one group.
func (b *RequestBuilder) Performance(perf PerformanceOptions) *RequestBuilder {
	b.req.Options.Performance = perf
	return b
}

// Build validates and returns the assembled Request.
func (b *RequestBuilder) Build() (Request, error) {
	if err := b.req.Validate(); err != nil {
		return Request{}, err
	}

	return b.req, nil
}
