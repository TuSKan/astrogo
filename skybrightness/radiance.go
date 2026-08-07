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
// satellite radiance and skyglow will need to be adjusted locally."
//
// Live-measured (2026-08-06), and root-caused, not just observed: for
// several city centres, this fit applied to a real VIIRS-DNB 2025 annual
// composite reads ~3x brighter than the same locations' World Atlas 2015
// (radiative-transfer-modelled) value. Cross-checking against
// lightpollutionmap.info's own live viirs_2025/wa_2015 layers at the same
// coordinates showed wa_2015 agreeing with this package's own decode within
// ~1-3%, but viirs_2025 only within ~10-30% at a MODERATE-brightness site —
// and diagnosing that gap by sampling raw radiance at neighbouring VIIRS
// pixels (±1-10 pixels, ~460 m-4.6 km) found the radiance there swinging
// from 0 to >6 nW·cm⁻²·sr⁻¹ within that radius, with the live API showing
// the identical zero/nonzero pixel pattern — i.e. this is real VIIRS-DNB
// per-pixel spatial noise (this package's decoder and the live API agree on
// WHICH pixels are dark), not a decode or georeferencing bug. The deeper
// reason: raw satellite-nadir radiance at one ~15-arcsec pixel is not the
// physical quantity zenith skyglow is — Falchi et al. 2016's atmospheric
// propagation model exists specifically because scattered light reaching an
// observer's zenith integrates contributions from sources up to ~300 km
// away, which single-pixel VIIRS radiance structurally cannot capture. This
// is why VIIRS is fine at a city core (many contiguous saturated-bright
// pixels — noise averages out) or a confirmed-dark remote site (already
// pinned to the detection floor), but genuinely imprecise — tens of percent,
// not a coefficient-tuning problem — at a moderate-brightness site whose
// neighbourhood has real pixel-to-pixel light-source heterogeneity, e.g. a
// rural property near scattered individual light sources. Real growth in
// detected artificial light 2015→2025 remains a separate, additional
// contributor to the ~3x city-centre gap, on top of this. LayerAuto tries
// VIIRS first regardless (freshness over fidelity, by design -- see its own
// doc comment) and returns it as soon as it succeeds, imprecise or not;
// request [skybrightness/atlas.LayerWorldAtlas] explicitly for the
// fidelity-first choice at exactly this class of site. The defaults below
// are therefore the ISS pair used as the closest published
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
