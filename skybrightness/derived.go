package skybrightness

import (
	"context"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/coord"
)

// PointQuery is the input to Point — the convenience half of this
// package's dual point/batch API (docs/skybrightness.md §5). Point exists
// so a caller that only wants one direction's brightness in one passband
// (the common plan-package use case) never has to build a full Request.
type PointQuery struct {
	Astro      *coord.Context
	Direction  coord.AltAz
	Passband   *Passband
	Mode       Mode
	Atmosphere *AtmosphereState
	// Grid is the spectral grid to evaluate on; the zero value substitutes
	// DefaultOpticalGrid().
	Grid SpectralGrid
}

// PointResult is Point's output: everything a caller typically needs from
// one direction, already reduced to scalars.
type PointResult struct {
	AB          SurfaceBrightnessAB
	Vega        SurfaceBrightnessVega // NaN when Passband has no VegaZeroPoint
	Radiance    Radiance
	Luminance   LuminanceCdM2
	Sigma       float64 // relative 1-sigma fraction; 0 when uncertainty wasn't computed
	AnthroRatio float64
	Quality     QualityFlags
	Provenance  Provenance
}

// Point evaluates e at one direction and reduces the result to scalars in
// PointQuery.Passband. Uncertainty is computed (UncLinearized) so
// PointResult.Sigma is meaningful; callers needing more control (batch
// evaluation, multiple passbands, materialized components) should call
// Engine.Evaluate/EvaluateBatch directly.
func Point(ctx context.Context, e Engine, q PointQuery) (PointResult, error) {
	grid := q.Grid
	if grid.Len() == 0 {
		grid = DefaultOpticalGrid()
	}

	var passbands []*Passband
	if q.Passband != nil {
		passbands = []*Passband{q.Passband}
	}

	res, err := e.Evaluate(ctx, Request{
		Astro:      q.Astro,
		Directions: []coord.AltAz{q.Direction},
		Grid:       grid,
		Passbands:  passbands,
		Mode:       q.Mode,
		Atmosphere: q.Atmosphere,
		Selection:  ComponentSelection{Materialize: false},
		Options: EvaluationOptions{
			Derived:     DerivePassbands | DeriveLuminance | DeriveAnthroRatio,
			Uncertainty: UncLinearized,
		},
	})
	if err != nil {
		return PointResult{}, fmt.Errorf("Point: %w", err)
	}

	out := PointResult{Quality: res.Quality, Provenance: res.Provenance}

	if q.Passband != nil {
		if r, err := IntegrateRadiance(grid, res.Total.Row(0), q.Passband); err == nil {
			out.Radiance = r
		}

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
				out.Vega = SurfaceBrightnessVega(math.NaN())
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
