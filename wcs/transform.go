package wcs

import (
	"fmt"
	"math"
)

const deg2rad = math.Pi / 180.0
const rad2deg = 180.0 / math.Pi

// PixelToWorld transforms a continuous FITS 1-indexed pixel coordinate
// slice mapping it against spherical Sky metrics.
// Currently implements baseline TAN (Gnomonic) spherical projections.
func (w *WCS) PixelToWorld(pixels []float64) ([]float64, error) {
	if len(pixels) != w.NAxis {
		return nil, fmt.Errorf("wcs: expected %d dimensional pixel input", w.NAxis)
	}

	// Step 1: Linear Transformation mapping offsets onto standard linear plane.
	inter := make([]float64, w.NAxis)
	offset := make([]float64, w.NAxis)
	for i := 0; i < w.NAxis; i++ {
		offset[i] = pixels[i] - w.CRPIX[i]
	}

	for i := 0; i < w.NAxis; i++ {
		var sum float64
		for j := 0; j < w.NAxis; j++ {
			sum += w.PC[i][j] * offset[j]
		}
		// Intermediate coordinates calculated mapping degrees over matrix skew.
		inter[i] = sum * w.CDELT[i]
	}

	// Step 2: Handle Native Astropy Spherical TAN Projection (Gnomonic)
	// Requires recognizing exactly 2 axes mapping standard Equatorial strings.
	if w.NAxis >= 2 && w.CTYPE[0] == "RA---TAN" && w.CTYPE[1] == "DEC--TAN" {
		xRad := inter[0] * deg2rad
		yRad := inter[1] * deg2rad

		alpha0 := w.CRVAL[0] * deg2rad
		delta0 := w.CRVAL[1] * deg2rad

		r := math.Hypot(xRad, yRad)
		if r == 0 {
			// Coordinate is mathematically equal to Reference Pixel bounds
			return []float64{w.CRVAL[0], w.CRVAL[1]}, nil
		}

		theta := math.Atan(1.0 / r)

		sinTheta, cosTheta := math.Sincos(theta)
		sinD0, cosD0 := math.Sincos(delta0)

		// Core Gnomonic Inverse Spherical Projection bounds natively computing offsets over curves.
		dec := math.Asin(cosTheta*sinD0 + (yRad/r)*sinTheta*cosD0)
		ra := alpha0 + math.Atan2(xRad*sinTheta, r*cosD0*cosTheta-yRad*sinD0*sinTheta)

		// Normalize RA mathematically into [0, 2pi] radius mappings natively scaling the sphere.
		if ra < 0 {
			ra += 2 * math.Pi
		}
		ra = math.Mod(ra, 2*math.Pi)

		// Inject back mapped values alongside scalar/dimensions.
		res := make([]float64, w.NAxis)
		res[0] = ra * rad2deg
		res[1] = dec * rad2deg

		for i := 2; i < w.NAxis; i++ {
			res[i] = w.CRVAL[i] + inter[i]
		}

		return res, nil
	}

	// Fallback mappings handling linear unsupported or raw data formats natively.
	res := make([]float64, w.NAxis)
	for i := 0; i < w.NAxis; i++ {
		res[i] = w.CRVAL[i] + inter[i]
	}

	return res, nil
}
