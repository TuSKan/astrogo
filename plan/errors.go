package plan

import "errors"

// Sentinel errors for observation planning.
var (
	// Event specification errors
	ErrNoPrimaryTarget   = errors.New("event spec must contain a primary target")
	ErrNoObserverLocation = errors.New("visibility events require an observer geodetic location")
	ErrNoSecondaryTarget = errors.New("geometry requires a secondary target")
	ErrFamilyNotImpl     = errors.New("event solver for family is not implemented")
	ErrUnsupportedGeom   = errors.New("unsupported geometry kind")
	ErrMoonRequired      = errors.New("illumination solver requires a Moon target")

	// FITS import errors
	ErrMissingSiteCoords = errors.New("plan/fits: missing mandatory SITELONG or SITELAT keywords")
	ErrMissingRA         = errors.New("plan/fits: missing CRVAL1 or RA_DEG mapping for RA coordinate")
	ErrMissingDec        = errors.New("plan/fits: missing CRVAL2 or DEC_DEG mapping for DEC coordinate")

	// Plan configuration errors
	ErrNotCoordObject    = errors.New("object does not implement coord.Object required for ranking")
	ErrStepNotPositive   = errors.New("step must be positive")
	ErrStepTooLarge      = errors.New("step exceeds maximum: large steps risk missing short visibility windows")
	ErrNotObservable     = errors.New("object does not implement Observable")
	ErrInvalidGeodetic   = errors.New("plan/fits: invalid geodetic location")

	// Solver errors
	ErrBracketingViolated = errors.New("solver: bracketing condition violated: f(a) and f(b) have the same sign")
)
