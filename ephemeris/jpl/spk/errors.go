package spk

import "errors"

// Sentinel errors for SPK operations.
var (
	// API errors
	ErrHorizonsBadRequest    = errors.New("jpl: horizons bad request (400): check keywords/content")
	ErrHorizonsMethodNA      = errors.New("jpl: horizons method not allowed (405)")
	ErrHorizonsServerError   = errors.New("jpl: horizons internal server error (500): database unavailable")
	ErrHorizonsUnavailable   = errors.New("jpl: horizons service unavailable (503): temporary overload/maintenance")
	ErrHorizonsUnexpected    = errors.New("jpl: horizons unexpected status")

	// Reader errors
	ErrCorruptSPK            = errors.New("jpl/spk: corrupt file")
	ErrInvalidWordBounds     = errors.New("jpl/spk: invalid double precision word bounds")
	ErrNoCoverage            = errors.New("jpl: no coverage for target")
	ErrUnsupportedSegment    = errors.New("jpl: unsupported SPK segment type")
	ErrInvalidRecordCount    = errors.New("jpl: invalid record count")
	ErrRecordTooShort        = errors.New("jpl: record too short")
	ErrInvalidOrder          = errors.New("jpl: polynomial order out of valid range")
)
