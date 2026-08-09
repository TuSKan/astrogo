package skybrightness

import (
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/unit"
)

// ComponentResults holds each evaluated component's SpectralField (when
// materialized) and ComponentReport, indexed by ComponentID via a
// fixed-size array — never a map, so Result's return path stays free of
// map/interface dispatch in the hot loop.
type ComponentResults struct {
	present ComponentMask
	fields  [numComponents]SpectralField
	reports [numComponents]ComponentReport
}

// Has reports whether component c was evaluated.
func (r *ComponentResults) Has(c ComponentID) bool { return r.present.Has(c) }

// Field returns component c's SpectralField and whether it was
// materialized (see ComponentSelection.Materialize).
func (r *ComponentResults) Field(c ComponentID) (SpectralField, bool) {
	if !r.present.Has(c) {
		return SpectralField{}, false
	}

	return r.fields[c], !r.fields[c].Empty()
}

// Report returns component c's ComponentReport.
func (r *ComponentResults) Report(c ComponentID) (ComponentReport, bool) {
	if !r.present.Has(c) {
		return ComponentReport{}, false
	}

	return r.reports[c], true
}

// Each calls fn for every evaluated component, in ComponentID order. fn
// returning false stops iteration early.
func (r *ComponentResults) Each(fn func(ComponentID, SpectralField, ComponentReport) bool) {
	for i := range numComponents {
		c := ComponentID(i)
		if !r.present.Has(c) {
			continue
		}

		if !fn(c, r.fields[c], r.reports[c]) {
			return
		}
	}
}

func (r *ComponentResults) set(c ComponentID, f SpectralField, rep ComponentReport) {
	r.present = r.present.Add(c)
	r.fields[c] = f
	r.reports[c] = rep
}

// PassbandResult is one Passband's derived per-direction brightness.
type PassbandResult struct {
	Passband PassbandID
	AB       []unit.SurfaceBrightnessAB
	Vega     []unit.SurfaceBrightnessVega // NaN-free only when the passband has a VegaZeroPoint
}

// DerivedQuantities carries every optional derived output an evaluation
// may compute, per EvaluationOptions.Derived. A field is populated only
// when its DerivedMask bit was requested; otherwise it stays nil/zero.
type DerivedQuantities struct {
	Passbands            []PassbandResult
	Luminance            []unit.LuminanceCdM2
	AnthroRatio          []float64 // artificial/natural passband-radiance ratio, per direction
	LimitingMagnitude    []float64 // per direction
	DetectorBackground   []unit.ElectronsPerPixelPerSecond
	MeanAllSky           unit.Radiance
	MedianAllSky         unit.Radiance
	BrightestDirection   int // index into Result.Directions; -1 if not computed
	HorizontalIrradiance unit.Irradiance
}

// Result is the outcome of one Evaluate call.
type Result struct {
	Grid       SpectralGrid
	Directions []coord.AltAz
	Total      SpectralField
	Components ComponentResults
	// Transmission is a flat, direction-major [nDir x nLambda] buffer of
	// atmospheric transmission, matching SpectralField's own layout —
	// empty unless Options.ComputeTransmission.
	Transmission []unit.Transmission
	Derived      DerivedQuantities
	Uncertainty  UncertaintyResult
	Provenance   Provenance
	Quality      QualityFlags
}

// BatchResult is the outcome of one EvaluateBatch call: one Result per
// epoch, plus a shared Provenance (per-epoch deltas, e.g. a fallback that
// occurred only for one epoch, live in that epoch's own
// Epochs[i].Provenance.Fallbacks).
type BatchResult struct {
	Epochs     []Result
	Provenance Provenance
}
