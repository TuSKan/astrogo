package kepler

import "errors"

// Sentinel errors for the kepler package.
var (
	// ErrUnsupportedOrbit indicates an eccentricity outside the
	// supported elliptical range [0, 1) — hyperbolic (e>1) and
	// parabolic (e=1) orbits are not implemented.
	ErrUnsupportedOrbit = errors.New("kepler: unsupported orbit (eccentricity must satisfy 0 <= e < 1)")
	// ErrInvalidElements indicates a non-finite or out-of-range orbital
	// element (semi-major axis, angle) that isn't specifically an
	// eccentricity problem.
	ErrInvalidElements = errors.New("kepler: invalid orbital elements")
	// ErrKeplerNoConverge indicates Kepler's equation failed to converge
	// to a finite eccentric anomaly within the iteration budget.
	ErrKeplerNoConverge = errors.New("kepler: eccentric-anomaly solve did not converge")
	// ErrSofaFailure indicates the underlying SOFA (gofaext) computation
	// backing the default base provider returned a failure status.
	ErrSofaFailure = errors.New("kepler: underlying SOFA computation failed")
	// ErrUnsupportedBody indicates a body ID the default SOFA base
	// provider (sofaBase) has no data for.
	ErrUnsupportedBody = errors.New("kepler: unsupported body for the default SOFA base provider")
)
