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
	// ErrKeplerNoConverge indicates the universal anomaly of a set built by
	// FromPerihelion failed to converge within the iteration budget. The
	// eccentric anomaly does not iterate (see SolveKepler).
	ErrKeplerNoConverge = errors.New("kepler: Kepler's equation did not converge")
	// ErrPhaseUnresolved indicates a mean anomaly whose phase a float64 cannot
	// resolve: not finite, or 2⁵² radians or more, where a float64 no longer
	// resolves an angle to a whole radian.
	ErrPhaseUnresolved = errors.New("kepler: mean anomaly too large to resolve its phase")
	// ErrSofaFailure indicates the underlying SOFA (gofaext) computation
	// backing the default base provider returned a failure status.
	ErrSofaFailure = errors.New("kepler: underlying SOFA computation failed")
	// ErrUnsupportedBody indicates a body ID the default SOFA base
	// provider (sofaBase) has no data for.
	ErrUnsupportedBody = errors.New("kepler: unsupported body for the default SOFA base provider")
)
