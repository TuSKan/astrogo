package atlas

import (
	"fmt"
	"math"
)

// pixelGetter returns the sample at integer pixel (col,row) and whether it is a
// valid (in-bounds, non-no-data) value.
type pixelGetter func(col, row int) (float64, bool)

// bilinear interpolates a sample at a longitude/latitude given an affine
// geotransform, raster dimensions, and a pixel accessor. No-data corners are
// dropped and the result is the weighted mean of the valid corners; all-no-data
// yields [ErrNoData] and an out-of-extent location yields [ErrOutOfCoverage].
//
// Both the in-memory [Grid] and the windowed GeoTIFF reader share this routine,
// so the interpolation and no-data handling are defined in exactly one place.
func bilinear(gt GeoTransform, width, height int, lonDeg, latDeg float64, at pixelGetter) (float64, error) {
	colF, rowF, ok := gt.pixelOf(lonDeg, latDeg)
	if !ok {
		return 0, fmt.Errorf("%w: degenerate geotransform", ErrInvalidGrid)
	}

	// GDAL pixel coordinates place the pixel centre at integer+0.5; shift so that
	// integer indices land on pixel centres for interpolation.
	colF -= 0.5
	rowF -= 0.5

	if colF < -0.5 || rowF < -0.5 || colF > float64(width)-0.5 || rowF > float64(height)-0.5 {
		return 0, fmt.Errorf("%w: lon=%.4f lat=%.4f", ErrOutOfCoverage, lonDeg, latDeg)
	}

	c0 := int(math.Floor(colF))
	r0 := int(math.Floor(rowF))
	fc := colF - float64(c0)
	fr := rowF - float64(r0)

	var sum, wsum float64

	for _, corner := range [4]struct {
		dc, dr int
		w      float64
	}{
		{0, 0, (1 - fc) * (1 - fr)},
		{1, 0, fc * (1 - fr)},
		{0, 1, (1 - fc) * fr},
		{1, 1, fc * fr},
	} {
		if v, valid := at(c0+corner.dc, r0+corner.dr); valid {
			sum += v * corner.w
			wsum += corner.w
		}
	}

	if wsum == 0 {
		return 0, fmt.Errorf("%w: lon=%.4f lat=%.4f", ErrNoData, lonDeg, latDeg)
	}

	return sum / wsum, nil
}
