package mpcorb

import "errors"

// Sentinel errors for the mpcorb package.
var (
	// ErrMalformedRow indicates an MPCORB row whose orbital elements are
	// not all numbers. Yielded for that row alone; iteration continues.
	ErrMalformedRow = errors.New("mpcorb: malformed element row")

	// ErrMalformedEpoch indicates a packed epoch that does not decode —
	// the wrong width, an unknown century marker, or a month or day
	// character outside the MPC's extended count.
	ErrMalformedEpoch = errors.New("mpcorb: malformed packed epoch")
)
