package plan

import "errors"

// Sentinel errors for FITS-to-plan header ingestion.
var (
	// ErrMissingSiteCoords indicates a header with none of the site keyword
	// sets SiteFromFITS reads.
	ErrMissingSiteCoords = errors.New("fits/plan: no observatory position: none of OBSGEO-X/Y/Z, OBSGEO-B/L/H, or SITELAT and SITELONG")
	// ErrMissingRA indicates a header with no right ascension: neither a
	// celestial WCS nor RA_DEG.
	ErrMissingRA = errors.New("fits/plan: no right ascension: neither a celestial WCS nor RA_DEG")
	// ErrMissingDec indicates a header with no declination: neither a
	// celestial WCS nor DEC_DEG.
	ErrMissingDec = errors.New("fits/plan: no declination: neither a celestial WCS nor DEC_DEG")
	// ErrUnsupportedFrame indicates celestial axes in a frame TargetFromFITS
	// does not convert: FK4, an equinox other than J2000, or ecliptic axes.
	ErrUnsupportedFrame = errors.New("fits/plan: celestial frame not supported")
	// ErrInvalidGeodetic indicates an invalid geodetic location in FITS import.
	ErrInvalidGeodetic = errors.New("fits/plan: invalid geodetic location")
)
