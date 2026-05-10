package fits

import "errors"

// Sentinel errors for FITS operations.
var (
	// BinTable errors
	ErrUninitBatch  = errors.New("fits: uninitialized bintable batch")
	ErrColumnNotFound = errors.New("fits: column not found")

	// Checksum errors
	ErrDatasumMismatch = errors.New("fits: DATASUM mismatch")

	// Reader errors
	ErrNoEndCard      = errors.New("fits: header exceeded max blocks without END card")
	ErrInvalidBitpix  = errors.New("fits: invalid BITPIX value")
	ErrEmptyHeader    = errors.New("fits verify: empty header")

	// MMap errors
	ErrInvalidWhence  = errors.New("mmapSeeker: invalid whence")
	ErrNegativeOffset = errors.New("mmapSeeker: negative offset")

	// WCS errors
	ErrWCSDimension     = errors.New("wcs: unexpected dimensional input")
	ErrWCSSingular      = errors.New("wcs: matrix is singular")
	ErrWCSNotConverged  = errors.New("wcs: WorldToPixel did not converge")
	ErrWCSBehindPlane   = errors.New("wcs: TAN projection: point behind tangent plane")
	ErrWCSAntipodal     = errors.New("wcs: AIT projection: antipodal point")
	ErrWCSOutsideSphere = errors.New("wcs: SIN projection: point outside unit sphere")
	ErrWCSOutsideRegion = errors.New("wcs: AIT projection: point outside valid region")
	ErrWCSUnsupported   = errors.New("wcs: unsupported projection")
	ErrWCSMissingNAXIS  = errors.New("fits/wcs: header missing mandatory NAXIS keyword")
	ErrWCSZeroDim       = errors.New("fits/wcs: header defines mathematically 0-dimensional plane")
)
