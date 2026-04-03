package wcs

// WCS defines the mathematical boundaries transforming image pixel matrices
// into absolute scientific world coordinates (e.g., Right Ascension / Declination mapping).
type WCS struct {
	NAxis int

	// CRPIX: Reference pixel coordinate array.
	// Note: FITS natively uses 1-based indexing for coordinates.
	// This system natively absorbs them verbatim.
	CRPIX []float64

	// CRVAL: The absolute world coordinate mapping exactly exactly onto the CRPIX Reference pixel.
	CRVAL []float64

	// CDELT: Linear spatial increment matrix scales at the reference pixel.
	CDELT []float64

	// CTYPE: Explicit coordinate axis type formats (e.g., "RA---TAN", "DEC--TAN") binding spherical projections.
	CTYPE []string

	// PC Matrix: Linear Transformation (rotation and skew) coefficients mapping
	// intermediate coordinates relative to the base increments.
	PC [][]float64
}

// New constructs an empty identity-mapped N-dimensional Coordinate System.
func New(naxis int) *WCS {
	pc := make([][]float64, naxis)
	for i := 0; i < naxis; i++ {
		pc[i] = make([]float64, naxis)
		pc[i][i] = 1.0 // Initialize with standard Identity matrix
	}

	return &WCS{
		NAxis: naxis,
		CRPIX: make([]float64, naxis),
		CRVAL: make([]float64, naxis),
		CDELT: make([]float64, naxis),
		CTYPE: make([]string, naxis),
		PC:    pc,
	}
}
