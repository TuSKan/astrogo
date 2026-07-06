package plan

import "errors"

// Sentinel errors for FITS-to-plan header ingestion.
var (
	// ErrMissingSiteCoords indicates missing SITELONG or SITELAT FITS keywords.
	ErrMissingSiteCoords = errors.New("fits/plan: missing mandatory SITELONG or SITELAT keywords")
	// ErrMissingRA indicates missing RA coordinate in FITS header.
	ErrMissingRA = errors.New("fits/plan: missing CRVAL1 or RA_DEG mapping for RA coordinate")
	// ErrMissingDec indicates missing DEC coordinate in FITS header.
	ErrMissingDec = errors.New("fits/plan: missing CRVAL2 or DEC_DEG mapping for DEC coordinate")
	// ErrInvalidGeodetic indicates an invalid geodetic location in FITS import.
	ErrInvalidGeodetic = errors.New("fits/plan: invalid geodetic location")
)
