package plan

import "errors"

// Sentinel errors for observation planning.
var (
	// ErrNoPrimaryTarget indicates an event spec missing its primary target.
	ErrNoPrimaryTarget = errors.New("event spec must contain a primary target")
	// ErrNoObserverLocation indicates visibility events require a geodetic location.
	ErrNoObserverLocation = errors.New("visibility events require an observer geodetic location")
	// ErrNoSecondaryTarget indicates a geometry event requires a secondary target.
	ErrNoSecondaryTarget = errors.New("geometry requires a secondary target")
	// ErrFamilyNotImpl indicates an event solver for the given family is not implemented.
	ErrFamilyNotImpl = errors.New("event solver for family is not implemented")
	// ErrUnsupportedGeom indicates an unsupported geometry kind.
	ErrUnsupportedGeom = errors.New("unsupported geometry kind")
	// ErrMoonRequired indicates the illumination solver requires a Moon target.
	ErrMoonRequired = errors.New("illumination solver requires a Moon target")

	// ErrMissingSiteCoords indicates missing SITELONG or SITELAT FITS keywords.
	ErrMissingSiteCoords = errors.New("plan/fits: missing mandatory SITELONG or SITELAT keywords")
	// ErrMissingRA indicates missing RA coordinate in FITS header.
	ErrMissingRA = errors.New("plan/fits: missing CRVAL1 or RA_DEG mapping for RA coordinate")
	// ErrMissingDec indicates missing DEC coordinate in FITS header.
	ErrMissingDec = errors.New("plan/fits: missing CRVAL2 or DEC_DEG mapping for DEC coordinate")

	// ErrNotCoordObject indicates the object does not implement coord.Object.
	ErrNotCoordObject = errors.New("object does not implement coord.Object required for ranking")
	// ErrStepNotPositive indicates a non-positive time step.
	ErrStepNotPositive = errors.New("step must be positive")
	// ErrStepTooLarge indicates a step that risks missing short visibility windows.
	ErrStepTooLarge = errors.New("step exceeds maximum: large steps risk missing short visibility windows")
	// ErrNotObservable indicates the object does not implement Observable.
	ErrNotObservable = errors.New("object does not implement Observable")
	// ErrInvalidGeodetic indicates an invalid geodetic location in FITS import.
	ErrInvalidGeodetic = errors.New("plan/fits: invalid geodetic location")

	// ErrBracketingViolated indicates f(a) and f(b) have the same sign in root finding.
	ErrBracketingViolated = errors.New("solver: bracketing condition violated: f(a) and f(b) have the same sign")
	// ErrEventNotFound indicates no event was found in the search window.
	ErrEventNotFound = errors.New("no event found in search window")
)
