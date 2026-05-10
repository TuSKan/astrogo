package spk

import "errors"

// Sentinel errors for SPK operations.
var (
	// ErrHorizonsBadRequest indicates a 400 response from JPL Horizons.
	ErrHorizonsBadRequest = errors.New("jpl: horizons bad request (400): check keywords/content")
	// ErrHorizonsMethodNA indicates a 405 response from JPL Horizons.
	ErrHorizonsMethodNA = errors.New("jpl: horizons method not allowed (405)")
	// ErrHorizonsServerError indicates a 500 response from JPL Horizons.
	ErrHorizonsServerError = errors.New("jpl: horizons internal server error (500): database unavailable")
	// ErrHorizonsUnavailable indicates a 503 response from JPL Horizons.
	ErrHorizonsUnavailable = errors.New("jpl: horizons service unavailable (503): temporary overload/maintenance")
	// ErrHorizonsUnexpected indicates an unexpected HTTP status from JPL Horizons.
	ErrHorizonsUnexpected = errors.New("jpl: horizons unexpected status")

	// ErrCorruptSPK indicates a malformed SPK binary kernel.
	ErrCorruptSPK = errors.New("jpl/spk: corrupt file")
	// ErrInvalidWordBounds indicates invalid double-precision word boundaries in an SPK record.
	ErrInvalidWordBounds = errors.New("jpl/spk: invalid double precision word bounds")
	// ErrNoCoverage indicates no ephemeris data covers the requested target/epoch.
	ErrNoCoverage = errors.New("jpl: no coverage for target")
	// ErrUnsupportedSegment indicates an SPK segment type that is not implemented.
	ErrUnsupportedSegment = errors.New("jpl: unsupported SPK segment type")
	// ErrInvalidRecordCount indicates an invalid record count in an SPK segment.
	ErrInvalidRecordCount = errors.New("jpl: invalid record count")
	// ErrRecordTooShort indicates an SPK record that is shorter than expected.
	ErrRecordTooShort = errors.New("jpl: record too short")
	// ErrInvalidOrder indicates a polynomial order outside the valid range.
	ErrInvalidOrder = errors.New("jpl: polynomial order out of valid range")
)
