package optics

import "errors"

// Sentinel errors for optics validation.
var (
	// ErrNonPositiveDimension indicates a physical dimension (aperture,
	// focal length, field-of-view, field-stop diameter) was zero,
	// negative, or non-finite.
	ErrNonPositiveDimension = errors.New("optics: dimension must be a positive, finite number")
	// ErrInvalidBarlowFactor indicates a Barlow/reducer factor was zero,
	// negative, or non-finite.
	ErrInvalidBarlowFactor = errors.New("optics: barlow/reducer factor must be a positive, finite number")
)
