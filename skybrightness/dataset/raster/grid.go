// Package raster decodes georeferenced single-band rasters — GeoTIFF today —
// into samples addressable by longitude and latitude.
//
// It is source-agnostic and carries no units: a sample is whatever the
// producing dataset says it is, and interpreting it is the caller's job. That
// separation matters here, because the datasets this package serves are
// satellite radiance products, and reading a satellite radiance as though it
// were sky brightness is the specific error
// [github.com/TuSKan/astrogo/skybrightness] exists to prevent.
//
// A large composite is read through a window rather than loaded whole: the
// VIIRS annual products run to gigabytes, and a caller usually wants a few
// hundred kilometres around one site.
package raster

import (
	"errors"
	"fmt"
	"math"
)

// Sentinel errors for raster decoding and sampling.
var (
	// ErrInvalidGrid is returned when a Grid is malformed — bad dimensions,
	// or a data length that does not match Width*Height.
	ErrInvalidGrid = errors.New("raster: invalid grid")

	// ErrOutOfCoverage is returned when a location falls outside the
	// raster's geographic extent.
	ErrOutOfCoverage = errors.New("raster: location outside coverage")

	// ErrNoData is returned when a location resolves only to no-data
	// pixels. It is distinct from a zero sample, which is a measurement.
	ErrNoData = errors.New("raster: no data at location")
)

// GeoTransform is a GDAL-style affine mapping from pixel and line to
// georeferenced longitude and latitude in degrees:
//
//	lon = A + col*B + row*C
//	lat = D + col*E + row*F
//
// For a north-up raster B is the +x pixel size, F the negative +y pixel size,
// and C = E = 0. Pixel centres sit at integer coordinates offset by +0.5, per
// GDAL's convention.
type GeoTransform struct {
	A, B, C, D, E, F float64
}

// pixelOf inverts the affine transform, returning fractional pixel
// coordinates for a longitude and latitude. ok is false for a degenerate
// transform.
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

// Grid is an in-memory single-band raster with an affine geotransform,
// row-major with row 0 at the top for a north-up transform.
//
// The samples carry no unit. Whatever produced the Grid knows what they mean.
type Grid struct {
	Width, Height int

	// Data holds Width*Height samples, row-major.
	Data []float64

	// NoData, when HasNoData is set, marks missing pixels.
	NoData    float64
	HasNoData bool

	GT GeoTransform
}

// Valid reports whether the dimensions and data length are consistent.
func (g *Grid) Valid() bool {
	return g != nil && g.Width > 0 && g.Height > 0 && len(g.Data) == g.Width*g.Height
}

// At returns the sample at integer pixel coordinates, and whether it is a
// valid in-bounds measurement.
func (g *Grid) At(col, row int) (float64, bool) {
	if col < 0 || row < 0 || col >= g.Width || row >= g.Height {
		return 0, false
	}

	v := g.Data[row*g.Width+col]
	if g.isNoData(v) {
		return 0, false
	}

	return v, true
}

// SampleBilinear returns the bilinearly interpolated sample at a longitude
// and latitude, sharing its interpolation and no-data handling with the
// windowed reader.
func (g *Grid) SampleBilinear(lonDeg, latDeg float64) (float64, error) {
	if !g.Valid() {
		return 0, fmt.Errorf("%w: %dx%d with %d samples",
			ErrInvalidGrid, gridWidth(g), gridHeight(g), gridLen(g))
	}

	return bilinear(g.GT, g.Width, g.Height, lonDeg, latDeg, g.At)
}

// LonLat returns the georeferenced centre of a pixel.
func (g *Grid) LonLat(col, row int) (lonDeg, latDeg float64) {
	c, r := float64(col)+0.5, float64(row)+0.5

	return g.GT.A + c*g.GT.B + r*g.GT.C, g.GT.D + c*g.GT.E + r*g.GT.F
}

// isNoData reports whether v is a no-data sample. NaN always is, whether or
// not the file declared a sentinel.
func (g *Grid) isNoData(v float64) bool {
	if math.IsNaN(v) {
		return true
	}

	return g.HasNoData && v == g.NoData
}

// Nil-safe accessors, so a malformed-grid error can report what it saw.
func gridWidth(g *Grid) int {
	if g == nil {
		return 0
	}

	return g.Width
}

func gridHeight(g *Grid) int {
	if g == nil {
		return 0
	}

	return g.Height
}

func gridLen(g *Grid) int {
	if g == nil {
		return 0
	}

	return len(g.Data)
}
