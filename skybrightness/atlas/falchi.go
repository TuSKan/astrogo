package atlas

import (
	"io"
	"sync"

	"github.com/TuSKan/astrogo/skybrightness"
)

// falchiConfig holds optional Falchi-loader settings.
type falchiConfig struct {
	override *GeoTransform
}

// FalchiOption configures the Falchi GeoTIFF loaders.
type FalchiOption func(*falchiConfig)

// WithGeoTransform supplies an affine geotransform to use when the GeoTIFF
// carries no model tags (ModelPixelScale/ModelTiepoint/ModelTransformation).
func WithGeoTransform(gt GeoTransform) FalchiOption {
	return func(c *falchiConfig) { c.override = &gt }
}

// tiffProvider is a windowed [skybrightness.SQMProvider] over a GeoTIFF. Its
// underlying reader keeps a one-block cache, so calls are serialized with a
// mutex; it is safe for concurrent use but not internally parallel.
type tiffProvider struct {
	mu sync.Mutex
	t  *geoTIFF
}

// NewFalchiProvider opens the Falchi et al. 2016 "World Atlas 2015" GeoTIFF
// (artificial zenith brightness, mcd/m²) for WINDOWED access: each query reads
// only the strips/tiles covering the location, so the ~2.9 GB global file is
// never loaded whole. The caller supplies the file as an [io.ReaderAt] (e.g. an
// *os.File); nothing is downloaded. See the package docs for the data DOI.
//
// Supported encodings: classic TIFF, 32/64-bit float samples, single band,
// uncompressed or deflate, no predictor, strip or tile layout. Other encodings
// return [ErrUnsupportedTIFF] (convert with, e.g.,
// `gdal_translate -ot Float32 -co COMPRESS=DEFLATE -co PREDICTOR=1`).
func NewFalchiProvider(r io.ReaderAt, opts ...FalchiOption) (skybrightness.SQMProvider, error) {
	var cfg falchiConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	t, err := openGeoTIFF(r, cfg.override)
	if err != nil {
		return nil, err
	}

	return &tiffProvider{t: t}, nil
}

// ZenithBrightness implements [skybrightness.SQMProvider].
func (p *tiffProvider) ZenithBrightness(latDeg, lonDeg float64) (skybrightness.SurfaceBrightnessV, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	mcd, err := p.t.sampleBilinear(lonDeg, latDeg)
	if err != nil {
		return 0, err
	}

	return skybrightness.SurfaceBrightnessFromMcdM2(mcd), nil
}

// LoadFalchiGrid decodes an entire (clipped/regional) Falchi GeoTIFF into an
// in-memory [Grid]. Use this for a pre-clipped tile; for the full global atlas
// prefer the windowed [NewFalchiProvider]. The resulting Grid can be wrapped
// with [NewGridProvider].
func LoadFalchiGrid(r io.ReaderAt, opts ...FalchiOption) (*Grid, error) {
	var cfg falchiConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	t, err := openGeoTIFF(r, cfg.override)
	if err != nil {
		return nil, err
	}

	return t.ReadGrid()
}
