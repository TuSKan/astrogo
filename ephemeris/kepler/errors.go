package kepler

import "errors"

// Sentinel errors for the kepler package.
var (
	// ErrUnsupportedOrbit indicates an eccentricity the element form cannot
	// represent: negative or not finite for either form, or e >= 1 for
	// NewElements, whose semi-major axis and mean anomaly describe an
	// ellipse only. FromPerihelion takes any e >= 0.
	ErrUnsupportedOrbit = errors.New("kepler: unsupported orbit")
	// ErrInvalidElements indicates a non-finite or out-of-range orbital
	// element (semi-major axis, angle) that isn't specifically an
	// eccentricity problem.
	ErrInvalidElements = errors.New("kepler: invalid orbital elements")
	// ErrKeplerNoConverge indicates Kepler's equation failed to converge
	// within the iteration budget: in the eccentric anomaly for a set built
	// by NewElements, or in the universal anomaly for one built by
	// FromPerihelion.
	ErrKeplerNoConverge = errors.New("kepler: Kepler's equation did not converge")
	// ErrSofaFailure indicates the underlying SOFA (gofaext) computation
	// backing the default base provider returned a failure status.
	ErrSofaFailure = errors.New("kepler: underlying SOFA computation failed")
	// ErrUnsupportedBody indicates a body ID the default SOFA base
	// provider (sofaBase) has no data for.
	ErrUnsupportedBody = errors.New("kepler: unsupported body for the default SOFA base provider")
)
