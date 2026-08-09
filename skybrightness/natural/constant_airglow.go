package natural

import (
	"context"
	"math"

	"github.com/TuSKan/astrogo/skybrightness"
	"github.com/TuSKan/astrogo/unit"
)

// Garstang nanolambert<->V-magnitude conversion constants, carried
// verbatim from astrogo v1's skybrightness/units.go. Unexported: only
// ConstantAirglow/VBandMoonlight may use them — this is
// explicitly NOT public API (docs/skybrightness.md §3's
// photometric-constants table).
//
// nlGarstangExp's coefficient 0.92104 is v1's own historically-published
// value, itself a 5-decimal rounding of Pogson's ratio in natural-log
// form, 0.4*ln(10) = 0.9210340371976184. Because garstangVegaZeroPoint
// (tophat_johnson.go) is defined as exactly
// nlGarstangScale*exp(nlGarstangExp) — the same literal 0.92104, not the
// more precise 0.4*ln(10) — the round trip
// -2.5*log10(garstangNanolambert(v)/garstangVegaZeroPoint) reproduces v to
// the precision of that shared, rounded literal (~1.5e-4 mag at V~22, the
// worst case in this package's test range), not to full float64
// precision. This is carried forward faithfully from v1's own rounding,
// not introduced here — see natural_test.go's
// TestConstantAirglow_RoundTripToHistoricalPrecision for the measured
// bound.
const (
	nlGarstangScale = 34.08
	nlGarstangExp   = 20.7233
)

// garstangNanolambert converts a V mag/arcsec^2 surface brightness to
// linear brightness in nanolamberts (Garstang's convention, as used by
// Krisciunas & Schaefer 1991 and Schaefer 1990).
func garstangNanolambert(v float64) float64 {
	return nlGarstangScale * math.Exp(nlGarstangExp-0.92104*v)
}

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
