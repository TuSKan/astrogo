package fits

import "errors"

// Sentinel errors for FITS operations.
var (
	// ErrUninitBatch indicates a bintable batch was used before initialization.
	ErrUninitBatch = errors.New("fits: uninitialized bintable batch")
	// ErrColumnNotFound indicates a requested column does not exist.
	ErrColumnNotFound = errors.New("fits: column not found")

	// ErrDatasumMismatch indicates a DATASUM verification failure.
	ErrDatasumMismatch = errors.New("fits: DATASUM mismatch")

	// ErrNoEndCard indicates the header exceeded maximum blocks without an END card.
	ErrNoEndCard = errors.New("fits: header exceeded max blocks without END card")
	// ErrInvalidBitpix indicates an unsupported BITPIX value.
	ErrInvalidBitpix = errors.New("fits: invalid BITPIX value")
	// ErrEmptyHeader indicates a FITS file with an empty header.
	ErrEmptyHeader = errors.New("fits verify: empty header")

	// ErrInvalidWhence indicates an invalid whence argument to Seek.
	ErrInvalidWhence = errors.New("mmapSeeker: invalid whence")
	// ErrNegativeOffset indicates a negative offset in Seek.
	ErrNegativeOffset = errors.New("mmapSeeker: negative offset")

	// ErrWCSDimension indicates unexpected dimensional input to a WCS transform.
	ErrWCSDimension = errors.New("wcs: unexpected dimensional input")
	// ErrWCSSingular indicates a singular WCS transformation matrix.
	ErrWCSSingular = errors.New("wcs: matrix is singular")
	// ErrWCSNotConverged indicates WorldToPixel iteration did not converge.
	ErrWCSNotConverged = errors.New("wcs: WorldToPixel did not converge")
	// ErrWCSBehindPlane indicates a point behind the TAN tangent plane.
	ErrWCSBehindPlane = errors.New("wcs: TAN projection: point behind tangent plane")
	// ErrWCSAntipodal indicates an antipodal point in AIT projection.
	ErrWCSAntipodal = errors.New("wcs: AIT projection: antipodal point")
	// ErrWCSOutsideSphere indicates a point outside the SIN unit sphere.
	ErrWCSOutsideSphere = errors.New("wcs: SIN projection: point outside unit sphere")
	// ErrWCSOutsideRegion indicates a point outside the AIT valid region.
	ErrWCSOutsideRegion = errors.New("wcs: AIT projection: point outside valid region")
	// ErrWCSUnsupported indicates an unsupported WCS projection type.
	ErrWCSUnsupported = errors.New("wcs: unsupported projection")
	// ErrWCSMissingNAXIS indicates a missing mandatory NAXIS keyword.
	ErrWCSMissingNAXIS = errors.New("fits/wcs: header missing mandatory NAXIS keyword")
	// ErrWCSZeroDim indicates a mathematically zero-dimensional WCS plane.
	ErrWCSZeroDim = errors.New("fits/wcs: header defines mathematically 0-dimensional plane")
)
