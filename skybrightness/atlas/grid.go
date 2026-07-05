package atlas

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/skybrightness"
)

// ErrInvalidGrid is returned when a [Grid] is malformed (bad dimensions or a
// data length that does not match Width×Height).
var ErrInvalidGrid = errors.New("atlas: invalid grid")

// ErrOutOfCoverage is returned when a queried location falls outside the grid's
// geographic extent.
var ErrOutOfCoverage = errors.New("atlas: location outside grid coverage")

// ErrNoData is returned when a queried location resolves only to no-data pixels.
var ErrNoData = errors.New("atlas: no data at location")

// GeoTransform is a GDAL-style affine mapping from pixel/line (col,row, with the
// pixel centre at integer coordinates offset by +0.5 in GDAL's convention) to
// georeferenced longitude/latitude in degrees:
//
//	lon = A + col·B + row·C
//	lat = D + col·E + row·F
//
// For a north-up raster B is the +x pixel size, F is the (negative) +y pixel
// size, and C = E = 0.
type GeoTransform struct {
	A, B, C, D, E, F float64
}

// pixelOf inverts the affine transform, returning fractional pixel coordinates
// (col,row) for a longitude/latitude. ok is false for a degenerate transform.
func (gt GeoTransform) pixelOf(lonDeg, latDeg float64) (col, row float64, ok bool) {
	det := gt.B*gt.F - gt.C*gt.E
	if det == 0 {
		return 0, 0, false
	}

	dx := lonDeg - gt.A
	dy := latDeg - gt.D
	col = (dx*gt.F - dy*gt.C) / det
	row = (dy*gt.B - dx*gt.E) / det

	return col, row, true
}

// Grid is an in-memory single-band raster of ARTIFICIAL sky brightness in
// mcd/m² (row-major, north-up or as described by GT), with an affine
// geotransform. It is the source-agnostic backbone every atlas loader produces.
type Grid struct {
	Width, Height int
	// Data holds Width×Height samples in mcd/m², row-major (row 0 at the top).
	Data []float64
	// NoData, when HasNoData is set, is the sentinel value marking missing pixels.
	NoData    float64
	HasNoData bool
	GT        GeoTransform
}

// valid reports whether the grid's dimensions and data length are consistent.
func (g *Grid) valid() bool {
	return g != nil && g.Width > 0 && g.Height > 0 && len(g.Data) == g.Width*g.Height
}

// isNoData reports whether v is a no-data sample.
func (g *Grid) isNoData(v float64) bool {
	if math.IsNaN(v) {
		return true
	}

	return g.HasNoData && v == g.NoData
}

// at returns the sample at integer pixel (col,row), or (0,false) if the pixel is
// out of bounds or no-data.
func (g *Grid) at(col, row int) (float64, bool) {
	if col < 0 || row < 0 || col >= g.Width || row >= g.Height {
		return 0, false
	}

	v := g.Data[row*g.Width+col]
	if g.isNoData(v) {
		return 0, false
	}

	return v, true
}

// sampleBilinear returns the bilinearly interpolated artificial brightness
// (mcd/m²) at a longitude/latitude, sharing the interpolation and no-data
// handling with the windowed GeoTIFF reader (see [bilinear]).
func (g *Grid) sampleBilinear(lonDeg, latDeg float64) (float64, error) {
	return bilinear(g.GT, g.Width, g.Height, lonDeg, latDeg, g.at)
}

// gridProvider adapts a [Grid] to [skybrightness.SQMProvider].
type gridProvider struct {
	g *Grid
}

// NewGridProvider returns an [skybrightness.SQMProvider] backed by a
// caller-decoded raster of artificial brightness in mcd/m². This is the
// source-agnostic path; the GeoTIFF loaders ([NewFalchiProvider]) ultimately
// build a Grid this consumes.
func NewGridProvider(g *Grid) (skybrightness.SQMProvider, error) {
	if !g.valid() {
		return nil, fmt.Errorf("%w: dims %dx%d, len(data)=%d", ErrInvalidGrid, safeDim(g), safeDimH(g), safeLen(g))
	}

	return gridProvider{g: g}, nil
}

func safeDim(g *Grid) int {
	if g == nil {
		return 0
	}

	return g.Width
}

func safeDimH(g *Grid) int {
	if g == nil {
		return 0
	}

	return g.Height
}

func safeLen(g *Grid) int {
	if g == nil {
		return 0
	}

	return len(g.Data)
}

// ZenithBrightness implements [skybrightness.SQMProvider]. It samples the
// artificial brightness at the location and converts mcd/m² → V mag/arcsec².
func (p gridProvider) ZenithBrightness(latDeg, lonDeg float64) (skybrightness.SurfaceBrightnessV, error) {
	mcd, err := p.g.sampleBilinear(lonDeg, latDeg)
	if err != nil {
		return 0, err
	}

	return skybrightness.SurfaceBrightnessFromMcdM2(mcd), nil
}
