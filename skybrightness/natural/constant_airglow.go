package natural

import (
	"context"

	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/unit"
)

// DefaultConstantAirglowV is the default dark-sky airglow + diffuse
// background floor, in V mag/arcsec^2 — ~21.9 is a representative
// true-night, high-altitude dark-sky value consistent with the Cerro
// Paranal sky model (Noll et al. 2012) and Patat (2008). Carried verbatim
// from astrogo v1.
const DefaultConstantAirglowV = 21.9

// ConstantAirglow is astrogo v1's airglow + diffuse-starlight floor
// component, re-implemented against the new Component interface: a
// constant V-band surface brightness, independent of direction, time,
// solar activity, and zenith angle — the honest, permanent distinction
// from the real spectral airglow model a future phase adds. See
// docs/skybrightness.md §15 — this is a brand-new type, not a
// compatibility shim.
type ConstantAirglow struct {
	sbV float64
}

// NewConstantAirglow creates a ConstantAirglow component using
// DefaultConstantAirglowV.
func NewConstantAirglow() *ConstantAirglow { return &ConstantAirglow{sbV: DefaultConstantAirglowV} }

// NewConstantAirglowSB creates a ConstantAirglow component with a
// caller-specified constant V-band surface brightness (mag/arcsec^2).
func NewConstantAirglowSB(sbV float64) *ConstantAirglow { return &ConstantAirglow{sbV: sbV} }

// ID implements skybrightness.Component.
func (a *ConstantAirglow) ID() skybrightness.ComponentID { return skybrightness.Airglow }

// Algorithm implements skybrightness.Component.
func (a *ConstantAirglow) Algorithm() skybrightness.AlgorithmRef {
	return skybrightness.AlgorithmRef{
		Name: "natural.ConstantAirglow", Version: "1.0.0",
		Citation: "Noll et al. (2012), A&A 543, A92; Patat (2008), A&A 481, 575",
	}
}

// Eval implements skybrightness.Component: writes a constant flat
// "spectrum" (Garstang nanolambert convention) across the top-hat V-band
// range, identical for every direction.
func (a *ConstantAirglow) Eval(_ context.Context, in skybrightness.EvalInput, out skybrightness.SpectralField) (skybrightness.ComponentReport, error) {
	nDir, _ := out.Dims()

	sb := a.sbV
	if sb == 0 {
		sb = DefaultConstantAirglowV
	}

	nl := unit.SpectralRadiance(garstangNanolambert(sb))

	values := make([]unit.SpectralRadiance, nDir)
	for i := range values {
		values[i] = nl
	}

	fillFlat(in.Grid, out, values)

	return skybrightness.ComponentReport{
		Assumptions: []string{
			"constant V-band airglow floor, no wavelength/solar-activity/zenith-angle/local-time dependence",
			"Garstang nanolambert convention, not SI radiance — meaningful only via VegaSurfaceBrightness against TopHatJohnsonV",
		},
		Uncertainty: skybrightness.ComponentUncertainty{RelSigma: 0.3, Group: skybrightness.GroupNatural, Kind: skybrightness.Epistemic},
		Provenance: skybrightness.ComponentProvenance{
			Component: skybrightness.Airglow, Algorithm: a.Algorithm(),
		},
		Quality: skybrightness.QualityFlagApproximatePhysics,
	}, nil
}
