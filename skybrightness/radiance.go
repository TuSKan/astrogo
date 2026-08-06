package skybrightness

import "math"

// Sánchez de Miguel et al. (2020), "The nature of the diffuse light near
// cities detected in nighttime satellite imagery", Sci. Rep. 10, 7829
// (https://doi.org/10.1038/s41598-020-64673-2), fit a log-linear relation
// between satellite night-light radiance L (nW·cm⁻²·sr⁻¹) and zenith sky
// brightness SB (mag/arcsec²):
//
//	SB = a·log₁₀(L) + b
//
// The paper publishes coefficients for two sensors — DMSP: a=−1.40±0.02,
// b=20.71±0.01 (R²=1, valid SB>19); ISS HDR: a=−1.71±0.1, b=20.00±0.05
// (R²=0.98, valid SB>18.5) — and shows VIIRS-DNB data only graphically
// (Fig. 9, against comparison lines of the ISS slope). No VIIRS-DNB-specific
// (a,b) pair exists in the literature: the paper cautions (Methods) that "as
// long as only broadband sensors are available, the correspondence between
// satellite radiance and skyglow will need to be adjusted locally." Live
// evidence of this gap (2026-08-06): for several city centres, this fit
// applied to a real VIIRS-DNB 2025 annual composite reads ~3x brighter than
// the same locations' World Atlas 2015 (radiative-transfer-modelled)
// value — cross-checked against lightpollutionmap.info's own live
// viirs_2025 layer, which agrees with this package's VIIRS decode within
// ~10-30% (vs. ~1-3% for the two sources' wa_2015 agreement), so the ~3x
// gap is real, not a decode bug — but it isn't fully attributable to this
// fit's miscalibration either: real growth in detected artificial light
// between 2015 and 2025 is a genuine, separate contributor, and the two
// cannot be cleanly separated without a proper VIIRS-DNB calibration. The
// defaults below are therefore the ISS pair used as the closest published
// broadband anchor — NOT a DNB calibration — for both this package's own
// VIIRS-layer consumers ([skybrightness/lpmap]) and
// [skybrightness/atlas]'s VIIRS loaders. Override via the slope/zeroPoint
// parameters (e.g. [skybrightness/atlas.WithVIIRSCoefficients]) once a
// DNB-calibrated pair is known.
const (
	// DefaultRadianceSlope is the ISS-HDR log-linear slope (a).
	DefaultRadianceSlope = -1.71
	// DefaultRadianceZeroPoint is the ISS-HDR log-linear zero-point (b).
	DefaultRadianceZeroPoint = 20.00
	// NaturalZenithMcdM2 is the natural (airglow + zodiacal + starlight)
	// zenith background, 0.171168465 mcd/m² ≡ 22.0 V mag/arcsec² (Falchi
	// et al. 2016; lightpollutionmap.info/help.html) — the single source
	// for this constant. Every sibling that needs it
	// ([skybrightness/atlas], [skybrightness/lpmap]) references this
	// symbol rather than re-declaring the literal. Subtracted from a
	// total-SB prediction to recover an artificial-only floor (see this
	// package's doc, §1.2 double-count warning), or added to an
	// artificial-only value to recover a total (e.g. before
	// [BortleClass], which expects a total observed brightness).
	NaturalZenithMcdM2 = 0.171168465
)

// RadianceToArtificialSB converts an upward night-light radiance
// (nW·cm⁻²·sr⁻¹, e.g. VIIRS-DNB) to an ARTIFICIAL-ONLY zenith surface
// brightness via the log-linear fit SB = slope·log₁₀(radiance) + zeroPoint,
// subtracting the natural background in linear luminance so the result
// composes with this package's other [Component] values without
// double-counting the natural sky. Non-positive radiance (no detected
// light) yields an infinitely faint artificial floor.
//
// This is an empirical, unpropagated fit — lower fidelity than a
// radiative-transfer-modelled atlas (e.g. Falchi et al. 2016); prefer that
// where available and use radiance data for freshness/trend analysis
// instead. See [DefaultRadianceSlope]/[DefaultRadianceZeroPoint] for the
// coefficients' provenance and caveats.
func RadianceToArtificialSB(radiance, slope, zeroPoint float64) SurfaceBrightnessV {
	if radiance <= 0 {
		return SurfaceBrightnessV(math.Inf(1))
	}

	totalSB := slope*math.Log10(radiance) + zeroPoint

	artificialMcd := SurfaceBrightnessV(totalSB).McdM2() - NaturalZenithMcdM2
	if artificialMcd <= 0 {
		return SurfaceBrightnessV(math.Inf(1))
	}

	return SurfaceBrightnessFromMcdM2(artificialMcd)
}
