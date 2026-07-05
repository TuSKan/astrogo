package atlas

import (
	"fmt"
	"io"
	"math"
	"sync"

	"github.com/TuSKan/astrogo/skybrightness"
)

// Sánchez de Miguel et al. (2020), "The nature of the diffuse light near cities
// detected in nighttime satellite imagery", Sci. Rep. 10, 7829
// (https://doi.org/10.1038/s41598-020-64673-2), fit a log-linear relation
// between satellite night-light radiance L (nW·cm⁻²·sr⁻¹) and zenith sky
// brightness SB (mag/arcsec²):
//
//	SB = a·log₁₀(L) + b
//
// The paper publishes coefficients for two sensors — DMSP: a=−1.40±0.02,
// b=20.71±0.01 (R²=1, valid SB>19); ISS HDR: a=−1.71±0.1, b=20.00±0.05 (R²=0.98,
// valid SB>18.5) — and shows VIIRS-DNB data only graphically (Fig. 9, against
// comparison lines of the ISS slope). The relation predicts the TOTAL observed
// zenith SB; this provider subtracts the natural background in linear space to
// return the ARTIFICIAL-only floor (see [github.com/TuSKan/astrogo/skybrightness],
// §1.2 double-count warning).
//
// TODO(verify): a VIIRS-DNB-specific (a,b). The paper publishes NO DNB
// coefficient pair, and cautions (Methods) that "as long as only broadband
// sensors are available, the correspondence between satellite radiance and
// skyglow will need to be adjusted locally". The defaults below are therefore
// the ISS pair used as the closest published broadband anchor — NOT a DNB
// calibration. Override with [WithVIIRSCoefficients] once a DNB-calibrated pair
// is known. This is exactly why the VIIRS floor is LOWER FIDELITY than the
// propagated WA/LPA atlases.
const (
	viirsSlope     = -1.71
	viirsZeroPoint = 20.00
	// viirsNaturalMcdM2 is the natural zenith background (mcd/m²) ≡ 22.0
	// mag/arcsec², subtracted to keep the floor artificial-only.
	viirsNaturalMcdM2 = 0.171168465
)

// viirsConfig holds optional VIIRS-loader settings.
type viirsConfig struct {
	override  *GeoTransform
	slope     float64
	zeroPoint float64
}

// VIIRSOption configures the VIIRS loader.
type VIIRSOption func(*viirsConfig)

// WithVIIRSGeoTransform supplies an affine geotransform for a GeoTIFF that
// carries no model tags.
func WithVIIRSGeoTransform(gt GeoTransform) VIIRSOption {
	return func(c *viirsConfig) { c.override = &gt }
}

// WithVIIRSCoefficients overrides the radiance→SB fit coefficients (slope a,
// zero-point b in SB = a·log₁₀(L) + b), e.g. with a VIIRS-DNB-calibrated pair.
func WithVIIRSCoefficients(slope, zeroPoint float64) VIIRSOption {
	return func(c *viirsConfig) { c.slope, c.zeroPoint = slope, zeroPoint }
}

// viirsProvider is a windowed [skybrightness.SQMProvider] over a VIIRS radiance
// GeoTIFF. Like [tiffProvider] it serializes access to the reader's one-block
// cache with a mutex.
type viirsProvider struct {
	mu        sync.Mutex
	t         *geoTIFF
	slope     float64
	zeroPoint float64
}

// NewVIIRSProvider opens a VIIRS annual-composite GeoTIFF (raw upward radiance,
// nW·cm⁻²·sr⁻¹ — e.g. VNP46A4 / VJ146A4 / EOG VNL) for windowed access and
// converts radiance to an ARTIFICIAL-only zenith sky brightness via the cited
// Sánchez de Miguel (2020) log-linear relation.
//
// FIDELITY WARNING: unlike [NewFalchiProvider] (Falchi 2016) and a future
// Lorenz loader, this source is NOT propagated through an atmospheric
// radiative-transfer model — it is a raw-radiance empirical fit. The
// correlation degrades at dark sites, where skyglow originates from distant
// sources the local pixel cannot capture, and the default coefficients are
// ISS-calibrated (see [WithVIIRSCoefficients]). Prefer WA/LPA for fidelity; use
// VIIRS for freshness (2024/2025) and trend analysis. The caller supplies the
// file; nothing is downloaded.
func NewVIIRSProvider(r io.ReaderAt, opts ...VIIRSOption) (skybrightness.SQMProvider, error) {
	cfg := viirsConfig{slope: viirsSlope, zeroPoint: viirsZeroPoint}
	for _, opt := range opts {
		opt(&cfg)
	}

	t, err := openGeoTIFF(r, cfg.override)
	if err != nil {
		return nil, err
	}

	return &viirsProvider{t: t, slope: cfg.slope, zeroPoint: cfg.zeroPoint}, nil
}

// ZenithBrightness implements [skybrightness.SQMProvider].
func (p *viirsProvider) ZenithBrightness(latDeg, lonDeg float64) (skybrightness.SurfaceBrightnessV, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	rad, err := p.t.sampleBilinear(lonDeg, latDeg)
	if err != nil {
		return 0, err
	}

	return radianceToArtificialSB(rad, p.slope, p.zeroPoint), nil
}

// viirsGridProvider applies the VIIRS radiance→SB fit over an in-memory [Grid]
// of radiance samples (e.g. decoded from an HDF5 granule via [LoadHDF5Grid]).
type viirsGridProvider struct {
	g         *Grid
	slope     float64
	zeroPoint float64
}

// NewVIIRSGridProvider returns a [skybrightness.SQMProvider] over a caller-loaded
// [Grid] of VIIRS radiance (nW·cm⁻²·sr⁻¹), applying the same empirical,
// lower-fidelity radiance→SQM conversion as [NewVIIRSProvider]. Only the
// coefficient option [WithVIIRSCoefficients] is meaningful here; a geotransform
// option is ignored (the grid carries its own).
func NewVIIRSGridProvider(g *Grid, opts ...VIIRSOption) (skybrightness.SQMProvider, error) {
	if !g.valid() {
		return nil, fmt.Errorf("%w: dims %dx%d, len(data)=%d", ErrInvalidGrid, safeDim(g), safeDimH(g), safeLen(g))
	}

	cfg := viirsConfig{slope: viirsSlope, zeroPoint: viirsZeroPoint}
	for _, opt := range opts {
		opt(&cfg)
	}

	return viirsGridProvider{g: g, slope: cfg.slope, zeroPoint: cfg.zeroPoint}, nil
}

// ZenithBrightness implements [skybrightness.SQMProvider].
func (p viirsGridProvider) ZenithBrightness(latDeg, lonDeg float64) (skybrightness.SurfaceBrightnessV, error) {
	rad, err := p.g.sampleBilinear(lonDeg, latDeg)
	if err != nil {
		return 0, err
	}

	return radianceToArtificialSB(rad, p.slope, p.zeroPoint), nil
}

// radianceToArtificialSB converts a VIIRS radiance (nW·cm⁻²·sr⁻¹) to an
// artificial-only zenith surface brightness. It applies the log-linear fit to
// get the TOTAL predicted SB, then subtracts the natural background in linear
// luminance so the result is artificial-only. Non-positive radiance (no
// detected light) yields an infinitely faint artificial floor.
func radianceToArtificialSB(radiance, slope, zeroPoint float64) skybrightness.SurfaceBrightnessV {
	if radiance <= 0 {
		return skybrightness.SurfaceBrightnessV(math.Inf(1))
	}

	totalSB := slope*math.Log10(radiance) + zeroPoint

	artificialMcd := skybrightness.SurfaceBrightnessV(totalSB).McdM2() - viirsNaturalMcdM2
	if artificialMcd <= 0 {
		return skybrightness.SurfaceBrightnessV(math.Inf(1))
	}

	return skybrightness.SurfaceBrightnessFromMcdM2(artificialMcd)
}
