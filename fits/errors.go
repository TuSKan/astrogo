package fits

import "errors"

// Sentinel errors for FITS operations.
var (
	// ErrBadTForm indicates a binary-table column declares a TFORM this
	// reader cannot lay out, or that the column widths disagree with the
	// declared row length.
	ErrBadTForm = errors.New("fits: unusable TFORM")
	// ErrUninitBatch indicates a bintable batch was used before initialization.
	ErrUninitBatch = errors.New("fits: uninitialized bintable batch")
	// ErrColumnNotFound indicates a requested column does not exist.
	ErrColumnNotFound = errors.New("fits: column not found")

	// ErrDatasumMismatch indicates a DATASUM verification failure.
	ErrDatasumMismatch = errors.New("fits: DATASUM mismatch")

	// ErrNoEndCard indicates the header ran past one of ReadHeader's two
	// failsafes without an END card — too many blocks read, or too many cards
	// retained. The wrapped message says which.
	ErrNoEndCard = errors.New("fits: header exceeded its size limit without an END card")
	// ErrInvalidBitpix indicates an unsupported BITPIX value.
	ErrInvalidBitpix = errors.New("fits: invalid BITPIX value")
	// ErrEmptyHeader indicates a FITS file with an empty header.
	ErrEmptyHeader = errors.New("fits verify: empty header")

	// ErrCardTooLong indicates a header card that does not fit the 80-byte
	// record: an over-long keyword, or a value and comment that together
	// overflow. It is an error rather than a truncation because the 80-byte
	// grid is the only thing separating one card from the next, so an
	// overflowing card shifts every card after it.
	ErrCardTooLong = errors.New("fits: header card exceeds 80 characters")

	// ErrCardNotPrintable indicates a header card carrying a byte outside
	// ASCII 32-126, which FITS does not permit in a header. A newline in a
	// comment corrupts the record grid rather than one card.
	ErrCardNotPrintable = errors.New("fits: header card contains a non-printable byte")

	// ErrNotWritable indicates an HDU this package cannot encode: an HDU kind
	// with no writer, a binary table asked to be the primary HDU, or an image
	// whose declared axes disagree with the data it carries.
	ErrNotWritable = errors.New("fits: HDU cannot be written")

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
