package coord

import (
	"fmt"
	"math"
)

// WCS defines the mathematical boundaries transforming image pixel matrices
// into absolute scientific world coordinates (e.g., Right Ascension / Declination mapping).
type WCS struct {
	nAxis int

	// crpix: Reference pixel coordinate array.
	// Note: FITS natively uses 1-based indexing for coordinates.
	// This system natively absorbs them verbatim.
	crpix []float64

	// crval: The absolute world coordinate mapping exactly exactly onto the CRPIX Reference pixel.
	crval []float64

	// cdelt: Linear spatial increment matrix scales at the reference pixel.
	cdelt []float64

	// ctype: Explicit coordinate axis type formats (e.g., "RA---TAN", "DEC--TAN") binding spherical projections.
	ctype []string

	// pc Matrix: Linear Transformation (rotation and skew) coefficients mapping
	// intermediate coordinates relative to the base increments.
	pc [][]float64
}

// New constructs an empty identity-mapped N-dimensional Coordinate System.
func NewWCS(naxis int) *WCS {
	pc := make([][]float64, naxis)
	for i := range naxis {
		pc[i] = make([]float64, naxis)
		pc[i][i] = 1.0 // Initialize with standard Identity matrix
	}

	return &WCS{
		nAxis: naxis,
		crpix: make([]float64, naxis),
		crval: make([]float64, naxis),
		cdelt: make([]float64, naxis),
		ctype: make([]string, naxis),
		pc:    pc,
	}
}

func (w *WCS) SetCRPIX(crpix []float64) {
	w.crpix = crpix
}

func (w *WCS) SetCRVAL(crval []float64) {
	w.crval = crval
}

func (w *WCS) SetCDELT(cdelt []float64) {
	w.cdelt = cdelt
}

func (w *WCS) SetCTYPE(ctype []string) {
	w.ctype = ctype
}

func (w *WCS) SetPC(pc [][]float64) {
	w.pc = pc
}

func (w *WCS) GetCRPIX() []float64 {
	return w.crpix
}

func (w *WCS) GetCRVAL() []float64 {
	return w.crval
}

func (w *WCS) GetCDELT() []float64 {
	return w.cdelt
}

func (w *WCS) GetCTYPE() []string {
	return w.ctype
}

func (w *WCS) GetPC() [][]float64 {
	return w.pc
}

const deg2rad = math.Pi / 180.0
const rad2deg = 180.0 / math.Pi

// PixelToWorld transforms a continuous FITS 1-indexed pixel coordinate
// slice mapping it against spherical Sky metrics.
// Currently implements baseline TAN (Gnomonic) spherical projections.
func (w *WCS) PixelToWorld(pixels []float64) ([]float64, error) {
	if len(pixels) != w.nAxis {
		return nil, fmt.Errorf("wcs: expected %d dimensional pixel input", w.nAxis)
	}

	// Step 1: Linear Transformation mapping offsets onto standard linear plane.
	inter := make([]float64, w.nAxis)
	offset := make([]float64, w.nAxis)
	for i := 0; i < w.nAxis; i++ {
		offset[i] = pixels[i] - w.crpix[i]
	}

	for i := 0; i < w.nAxis; i++ {
		var sum float64
		for j := 0; j < w.nAxis; j++ {
			sum += w.pc[i][j] * offset[j]
		}
		// Intermediate coordinates calculated mapping degrees over matrix skew.
		inter[i] = sum * w.cdelt[i]
	}

	// Step 2: Handle Native Astropy Spherical TAN Projection (Gnomonic)
	// Requires recognizing exactly 2 axes mapping standard Equatorial strings.
	if w.nAxis >= 2 && w.ctype[0] == "RA---TAN" && w.ctype[1] == "DEC--TAN" {
		xRad := inter[0] * deg2rad
		yRad := inter[1] * deg2rad

		alpha0 := w.crval[0] * deg2rad
		delta0 := w.crval[1] * deg2rad

		r := math.Hypot(xRad, yRad)
		if r == 0 {
			// Coordinate is mathematically equal to Reference Pixel bounds
			return []float64{w.crval[0], w.crval[1]}, nil
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
		res := make([]float64, w.nAxis)
		res[0] = ra * rad2deg
		res[1] = dec * rad2deg

		for i := 2; i < w.nAxis; i++ {
			res[i] = w.crval[i] + inter[i]
		}

		return res, nil
	}

	// Fallback mappings handling linear unsupported or raw data formats natively.
	res := make([]float64, w.nAxis)
	for i := 0; i < w.nAxis; i++ {
		res[i] = w.crval[i] + inter[i]
	}

	return res, nil
}
