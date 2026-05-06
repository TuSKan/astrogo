package fits

import (
	"fmt"
	"math"
	"strings"
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

// NewWCS constructs an empty identity-mapped N-dimensional Coordinate System.
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

// SetCRPIX sets the reference pixel coordinate array (1-based FITS indexing).
func (w *WCS) SetCRPIX(crpix []float64) {
	w.crpix = crpix
}

// SetCRVAL sets the world coordinate values at the reference pixel.
func (w *WCS) SetCRVAL(crval []float64) {
	w.crval = crval
}

// SetCDELT sets the pixel scale (degrees per pixel) along each axis.
func (w *WCS) SetCDELT(cdelt []float64) {
	w.cdelt = cdelt
}

// SetCTYPE sets the coordinate axis type identifiers (e.g., "RA---TAN", "DEC--TAN").
func (w *WCS) SetCTYPE(ctype []string) {
	w.ctype = ctype
}

// SetPC sets the linear transformation (rotation/skew) matrix.
func (w *WCS) SetPC(pc [][]float64) {
	w.pc = pc
}

// GetCRPIX returns the reference pixel coordinate array.
func (w *WCS) GetCRPIX() []float64 {
	return w.crpix
}

// GetCRVAL returns the world coordinate values at the reference pixel.
func (w *WCS) GetCRVAL() []float64 {
	return w.crval
}

// GetCDELT returns the pixel scale along each axis.
func (w *WCS) GetCDELT() []float64 {
	return w.cdelt
}

// GetCTYPE returns the coordinate axis type identifiers.
func (w *WCS) GetCTYPE() []string {
	return w.ctype
}

// GetPC returns the linear transformation matrix.
func (w *WCS) GetPC() [][]float64 {
	return w.pc
}

const deg2rad = math.Pi / 180.0
const rad2deg = 180.0 / math.Pi

// PixelToWorld transforms a continuous FITS 1-indexed pixel coordinate
// slice mapping it against spherical Sky metrics.
//
// Supported projections: TAN (Gnomonic), SIN (Orthographic),
// ARC (Zenithal equidistant), STG (Stereographic), AIT (Hammer-Aitoff).
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
		inter[i] = sum * w.cdelt[i]
	}

	// Step 2: Spherical de-projection if celestial axes are present.
	proj := w.extractProjection()
	if w.nAxis >= 2 && proj != "" {
		xRad := inter[0] * deg2rad
		yRad := inter[1] * deg2rad

		alpha0 := w.crval[0] * deg2rad
		delta0 := w.crval[1] * deg2rad

		ra, dec, err := deproject(proj, xRad, yRad, alpha0, delta0)
		if err != nil {
			return nil, err
		}

		// Normalize RA into [0, 2π).
		if ra < 0 {
			ra += 2 * math.Pi
		}
		ra = math.Mod(ra, 2*math.Pi)

		res := make([]float64, w.nAxis)
		res[0] = ra * rad2deg
		res[1] = dec * rad2deg
		for i := 2; i < w.nAxis; i++ {
			res[i] = w.crval[i] + inter[i]
		}
		return res, nil
	}

	// Fallback: linear mapping.
	res := make([]float64, w.nAxis)
	for i := 0; i < w.nAxis; i++ {
		res[i] = w.crval[i] + inter[i]
	}
	return res, nil
}

// WorldToPixel converts world coordinates back to FITS 1-indexed pixel coordinates
// using Newton-Raphson iteration on PixelToWorld.
//
// Convergence is typically 3–5 iterations for well-conditioned projections.
// Returns an error if the solver does not converge within 20 iterations
// or if the Jacobian becomes singular.
func (w *WCS) WorldToPixel(world []float64) ([]float64, error) {
	if len(world) != w.nAxis {
		return nil, fmt.Errorf("wcs: expected %d dimensional world input", w.nAxis)
	}

	// Initial guess: linear inverse.
	pix := make([]float64, w.nAxis)
	for i := 0; i < w.nAxis; i++ {
		pix[i] = w.crpix[i] + (world[i]-w.crval[i])/w.cdelt[i]
	}

	const maxIter = 20
	const tol = 1e-12 // degrees (~0.036 mas)

	for iter := 0; iter < maxIter; iter++ {
		fwd, err := w.PixelToWorld(pix)
		if err != nil {
			return nil, fmt.Errorf("wcs: WorldToPixel forward evaluation failed: %w", err)
		}

		// Residual = fwd - world
		dx := fwd[0] - world[0]
		dy := fwd[1] - world[1]

		// Handle RA wrap-around: pick shortest arc
		if dx > 180 {
			dx -= 360
		} else if dx < -180 {
			dx += 360
		}

		if math.Abs(dx) < tol && math.Abs(dy) < tol {
			return pix, nil
		}

		// Numerical Jacobian via finite differences
		const h = 1e-6
		pix1 := make([]float64, w.nAxis)
		copy(pix1, pix)
		pix1[0] += h
		f1, err := w.PixelToWorld(pix1)
		if err != nil {
			return nil, err
		}

		pix2 := make([]float64, w.nAxis)
		copy(pix2, pix)
		pix2[1] += h
		f2, err := w.PixelToWorld(pix2)
		if err != nil {
			return nil, err
		}

		// J = [[df0/dp0, df0/dp1], [df1/dp0, df1/dp1]]
		j00 := (f1[0] - fwd[0]) / h
		j01 := (f2[0] - fwd[0]) / h
		j10 := (f1[1] - fwd[1]) / h
		j11 := (f2[1] - fwd[1]) / h

		det := j00*j11 - j01*j10
		if math.Abs(det) < 1e-30 {
			return nil, fmt.Errorf("wcs: WorldToPixel Jacobian is singular")
		}

		// Newton update: pix -= J^{-1} * residual
		pix[0] -= (j11*dx - j01*dy) / det
		pix[1] -= (-j10*dx + j00*dy) / det
	}

	return nil, fmt.Errorf("wcs: WorldToPixel did not converge after %d iterations", maxIter)
}

// ── Projection Helpers ───────────────────────────────────────────────────────

// extractProjection returns the 3-letter projection code from CTYPE (e.g., "TAN").
// Returns "" if no recognized celestial projection is found.
func (w *WCS) extractProjection() string {
	if w.nAxis < 2 {
		return ""
	}
	ct0 := w.ctype[0]
	if len(ct0) >= 8 {
		proj := ct0[5:8]
		switch proj {
		case "TAN", "SIN", "ARC", "STG", "AIT":
			return proj
		}
	}
	return ""
}

// deproject applies the inverse spherical projection to recover (ra, dec) in radians
// from intermediate coordinates (x, y) in radians relative to the reference point.
func deproject(proj string, x, y, alpha0, delta0 float64) (ra, dec float64, err error) {
	sinD0, cosD0 := math.Sincos(delta0)

	switch proj {
	case "TAN":
		r := math.Hypot(x, y)
		if r == 0 {
			return alpha0, delta0, nil
		}
		theta := math.Atan(1.0 / r)
		sinT, cosT := math.Sincos(theta)
		dec = math.Asin(cosT*sinD0 + (y/r)*sinT*cosD0)
		ra = alpha0 + math.Atan2(x*sinT, r*cosD0*cosT-y*sinD0*sinT)

	case "SIN":
		// Orthographic (slant-projection variant with xi=0, eta=0)
		r2 := x*x + y*y
		if r2 > 1.0 {
			return 0, 0, fmt.Errorf("wcs: SIN projection: point outside unit sphere (r²=%.6f)", r2)
		}
		cosT := math.Sqrt(1 - r2)
		dec = math.Asin(cosT*sinD0 + y*cosD0)
		ra = alpha0 + math.Atan2(x, cosD0*cosT-y*sinD0)

	case "ARC":
		// Zenithal equidistant
		r := math.Hypot(x, y)
		if r == 0 {
			return alpha0, delta0, nil
		}
		theta := math.Pi/2 - r
		sinT, cosT := math.Sincos(theta)
		dec = math.Asin(cosT*sinD0 + (y/r)*sinT*cosD0)
		ra = alpha0 + math.Atan2(x*sinT, r*cosD0*cosT-y*sinD0*sinT)

	case "STG":
		// Stereographic
		r := math.Hypot(x, y)
		if r == 0 {
			return alpha0, delta0, nil
		}
		theta := math.Pi/2 - 2*math.Atan(r/2)
		sinT, cosT := math.Sincos(theta)
		dec = math.Asin(cosT*sinD0 + (y/r)*sinT*cosD0)
		ra = alpha0 + math.Atan2(x*sinT, r*cosD0*cosT-y*sinD0*sinT)

	case "AIT":
		// Hammer-Aitoff (full-sky equal-area)
		z2 := 1.0 - (x/4)*(x/4) - (y/2)*(y/2)
		if z2 < 0 {
			return 0, 0, fmt.Errorf("wcs: AIT projection: point outside valid region")
		}
		z := math.Sqrt(z2)
		dec = math.Asin(y * z)
		ra = alpha0 + 2*math.Atan2(x*z, 2*(2*z2-1))

	default:
		return 0, 0, fmt.Errorf("wcs: unsupported projection %q", proj)
	}

	return ra, dec, nil
}

// ── FITS Header Extraction ───────────────────────────────────────────────────

// ExtractWCS dynamically translates FITS standard header layouts structurally matching
// coordinate metrics arrays natively mapping the resulting abstractions into [WCS].
func ExtractWCS(h *Header) (*WCS, error) {
	naxis, err := h.GetInt("NAXIS")
	if err != nil {
		return nil, fmt.Errorf("fits/wcs: header missing mandatory NAXIS keyword")
	}
	if naxis <= 0 {
		return nil, fmt.Errorf("fits/wcs: header defines mathematically 0-dimensional plane")
	}

	crpix := make([]float64, naxis)
	crval := make([]float64, naxis)
	cdelt := make([]float64, naxis)
	ctype := make([]string, naxis)
	pc := make([][]float64, naxis)

	for i := 1; i <= naxis; i++ {
		idx := i - 1

		c, _ := h.GetString(fmt.Sprintf("CTYPE%d", i))
		ctype[idx] = strings.TrimSpace(c)

		if v, err := h.GetFloat(fmt.Sprintf("CRVAL%d", i)); err == nil {
			crval[idx] = v
		}

		if p, err := h.GetFloat(fmt.Sprintf("CRPIX%d", i)); err == nil {
			crpix[idx] = p
		}

		d, err := h.GetFloat(fmt.Sprintf("CDELT%d", i))
		if err != nil {
			d = 1.0
		}
		cdelt[idx] = d

		pc[idx] = make([]float64, naxis)
	}

	// Try CDi_j first (used by HST, JWST, DES, LSST, and most survey pipelines).
	// If any CDi_j keyword is found, use the CD matrix convention exclusively.
	hasCDMatrix := false
	cd := make([][]float64, naxis)
	for i := 1; i <= naxis; i++ {
		cd[i-1] = make([]float64, naxis)
		for j := 1; j <= naxis; j++ {
			val, err := h.GetFloat(fmt.Sprintf("CD%d_%d", i, j))
			if err == nil {
				hasCDMatrix = true
				cd[i-1][j-1] = val
			}
		}
	}

	if hasCDMatrix {
		// Decompose CD matrix: CDi_j = CDELTi * PCi_j
		// Extract CDELT as column norms and PC as the normalized rotation.
		for i := 0; i < naxis; i++ {
			var colNorm float64
			for j := 0; j < naxis; j++ {
				colNorm += cd[j][i] * cd[j][i]
			}
			colNorm = math.Sqrt(colNorm)
			if colNorm == 0 {
				colNorm = 1.0 // Avoid division by zero for degenerate axes
			}

			// Preserve sign from the diagonal element
			if cd[i][i] < 0 {
				cdelt[i] = -colNorm
			} else {
				cdelt[i] = colNorm
			}

			for j := 0; j < naxis; j++ {
				pc[j][i] = cd[j][i] / cdelt[i]
			}
		}
	} else {
		// Fall back to PCi_j + CDELT convention.
		for i := 1; i <= naxis; i++ {
			for j := 1; j <= naxis; j++ {
				val, err := h.GetFloat(fmt.Sprintf("PC%d_%d", i, j))
				if err == nil {
					pc[i-1][j-1] = val
				} else if i == j {
					pc[i-1][j-1] = 1.0
				}
			}
		}
	}

	w := NewWCS(naxis)
	w.SetCTYPE(ctype)
	w.SetCRVAL(crval)
	w.SetCRPIX(crpix)
	w.SetCDELT(cdelt)
	w.SetPC(pc)

	return w, nil
}
