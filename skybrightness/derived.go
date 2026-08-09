package skybrightness

import (
	"context"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/unit"
)

// PointQuery is the input to Point — the convenience half of this
// package's dual point/batch API (docs/skybrightness.md §5). Point exists
// so a caller that only wants one direction's brightness in one passband
// (the common plan-package use case) never has to build a full Request or
// touch Engine.Evaluate directly — including for transmission and limiting
// magnitude, the two derived quantities that used to force a caller back
// onto Evaluate (see docs/skybrightness.md §15's worked example).
type PointQuery struct {
	Astro      *coord.Context
	Direction  coord.AltAz
	Passband   *Passband
	Mode       Mode
	Atmosphere *atmosphere.Atmosphere
	// Grid is the spectral grid to evaluate on; the zero value substitutes
	// DefaultOpticalGrid().
	Grid SpectralGrid
	// Components, when true, additionally computes each evaluated
	// component's own passband-integrated radiance (PointResult.Components)
	// — the per-component breakdown a caller inspecting *why* the sky is
	// as bright as it is wants, without dropping to Engine.Evaluate
	// directly. Costs one extra SpectralField per component
	// (Selection.Materialize:true) versus the false default.
	Components bool
	// ComputeTransmission, when true, additionally computes line-of-sight
	// transmission (PointResult.Transmission), when the engine has a
	// TransmissionModel configured.
	ComputeTransmission bool
	// LimitingMag, when non-nil, additionally computes a limiting
	// magnitude (PointResult.LimitingMagnitude/HasLimitingMag) via the
	// given model. Requires Passband to be set — DeriveLimitingMag is a
	// per-passband derived quantity.
	LimitingMag LimitingMagModel
}

// ComponentBrightness is one component's own passband-integrated
// radiance, alongside its self-reported uncertainty and quality —
// PointResult.Components, populated only when PointQuery.Components is
// true.
type ComponentBrightness struct {
	ID       ComponentID
	Radiance unit.Radiance
	RelSigma float64
	Quality  QualityFlags
}

// PointResult is Point's output: everything a caller typically needs from
// one direction, already reduced to scalars.
type PointResult struct {
	AB          unit.SurfaceBrightnessAB
	Vega        unit.SurfaceBrightnessVega // NaN when Passband has no VegaZeroPoint
	Radiance    unit.Radiance
	Luminance   unit.LuminanceCdM2
	Sigma       float64 // relative 1-sigma fraction; 0 when uncertainty wasn't computed
	AnthroRatio float64
	Quality     QualityFlags
	Provenance  Provenance
	// Components is the per-component passband-integrated breakdown, in
	// ComponentID order — nil unless PointQuery.Components was true.
	Components []ComponentBrightness
	// Transmission is the per-wavelength line-of-sight transmission,
	// aligned to the evaluation grid — nil unless
	// PointQuery.ComputeTransmission was true.
	Transmission []unit.Transmission
	// LimitingMagnitude is the derived limiting magnitude for
	// PointQuery.Passband, valid only when HasLimitingMag is true — which
	// requires both PointQuery.LimitingMag and PointQuery.Passband to have
	// been set.
	LimitingMagnitude float64
	HasLimitingMag    bool
}

// Point evaluates e at one direction and reduces the result to scalars in
// PointQuery.Passband. Uncertainty is computed (UncLinearized) so
// PointResult.Sigma is meaningful; callers needing more control (batch
// evaluation, multiple passbands, arbitrary derived-quantity combinations)
// should call Engine.Evaluate/EvaluateBatch directly.
func Point(ctx context.Context, e Engine, q PointQuery) (PointResult, error) {
	grid := q.Grid
	if grid.Len() == 0 {
		grid = DefaultOpticalGrid()
	}

	var passbands []*Passband
	if q.Passband != nil {
		passbands = []*Passband{q.Passband}
	}

	b := NewRequestBuilder(q.Astro, []coord.AltAz{q.Direction}, grid).
		Passbands(passbands...).
		Mode(q.Mode).
		Atmosphere(q.Atmosphere).
		Derive(DerivePassbands|DeriveLuminance|DeriveAnthroRatio).
		Uncertainty(UncLinearized, 0, 0)

	if q.Components {
		b = b.Materialize()
	}

	if q.ComputeTransmission {
		b = b.Transmission()
	}

	if q.LimitingMag != nil {
		b = b.LimitingMag(q.LimitingMag)
	}

	req, err := b.Build()
	if err != nil {
		return PointResult{}, fmt.Errorf("Point: %w", err)
	}

	res, err := e.Evaluate(ctx, req)
	if err != nil {
		return PointResult{}, fmt.Errorf("Point: %w", err)
	}

	out := PointResult{Quality: res.Quality, Provenance: res.Provenance}

	if q.ComputeTransmission {
		out.Transmission = res.Transmission
	}

	if q.LimitingMag != nil && len(res.Derived.LimitingMagnitude) > 0 {
		out.LimitingMagnitude = res.Derived.LimitingMagnitude[0]
		out.HasLimitingMag = true
	}

	if q.Components {
		var integrateErr error

		res.Components.Each(func(id ComponentID, f SpectralField, rep ComponentReport) bool {
			cb := ComponentBrightness{ID: id, RelSigma: rep.Uncertainty.RelSigma, Quality: rep.Quality}

			if q.Passband != nil && !f.Empty() {
				r, err := IntegrateRadiance(grid, f.Row(0), q.Passband)
				if err != nil {
					integrateErr = fmt.Errorf("Point: integrate component %v radiance: %w", id, err)
					return false
				}

				cb.Radiance = r
			}

			out.Components = append(out.Components, cb)

			return true
		})

		if integrateErr != nil {
			return PointResult{}, integrateErr
		}
	}

	if q.Passband != nil {
		r, err := IntegrateRadiance(grid, res.Total.Row(0), q.Passband)
		if err != nil {
			return PointResult{}, fmt.Errorf("Point: integrate total radiance: %w", err)
		}

		out.Radiance = r

		for _, pr := range res.Derived.Passbands {
			if pr.Passband != q.Passband.ID {
				continue
			}

			if len(pr.AB) > 0 {
				out.AB = pr.AB[0]
			}

			if len(pr.Vega) > 0 {
				out.Vega = pr.Vega[0]
			} else {
				out.Vega = unit.SurfaceBrightnessVega(math.NaN())
			}
		}
	}

	if len(res.Derived.Luminance) > 0 {
		out.Luminance = res.Derived.Luminance[0]
	}

	if len(res.Derived.AnthroRatio) > 0 {
		out.AnthroRatio = res.Derived.AnthroRatio[0]
	}

	if n := res.Uncertainty.RelSigma; !n.Empty() {
		out.Sigma = float64(n.At(0, 0))
	}

	return out, nil
}
