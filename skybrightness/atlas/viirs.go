package atlas

import (
	"fmt"
	"io"
	"sync"

	"github.com/TuSKan/astrogo/skybrightness"
)

// The radiance→SB conversion (Sánchez de Miguel et al. 2020's log-linear
// fit) lives in [skybrightness.RadianceToArtificialSB] — shared with
// [github.com/TuSKan/astrogo/skybrightness/lpmap]'s VIIRS-layer handling,
// which needs the identical conversion for a live-queried radiance value.
// See that function's doc for the full provenance/caveats, including the
// still-unresolved TODO(verify): no VIIRS-DNB-specific (a,b) pair exists in
// the literature — [DefaultRadianceSlope]/[DefaultRadianceZeroPoint] below
// are the closest published broadband (ISS-HDR) anchor, not a DNB
// calibration. This is exactly why the VIIRS floor is LOWER FIDELITY than
// the propagated WA/LPA atlases.
const (
	viirsSlope     = skybrightness.DefaultRadianceSlope
	viirsZeroPoint = skybrightness.DefaultRadianceZeroPoint
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

	return skybrightness.RadianceToArtificialSB(rad, p.slope, p.zeroPoint), nil
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

	return skybrightness.RadianceToArtificialSB(rad, p.slope, p.zeroPoint), nil
}
