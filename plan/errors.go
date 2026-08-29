package plan

import "errors"

// Sentinel errors for observation planning.
var (
	// ErrInvalidHorizon is returned when the horizon is outside the valid range.
	ErrInvalidHorizon = errors.New("horizon must be between -90 and 90 degrees")
	// ErrNilLocation is returned when the geodetic location is nil.
	ErrNilLocation = errors.New("geodetic location must not be nil")

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
	// ErrNoSkyDepth indicates a LimitingMagnitudeConstraint with no SkyDepth
	// to ask. There is no default and there cannot be one: a limiting
	// magnitude depends on the detector, the exposure and the observer.
	ErrNoSkyDepth = errors.New("limiting magnitude constraint requires a SkyDepth")

	// ErrNotCoordObject indicates the object does not implement coord.Object.
	//
	// Deprecated: RankObservable no longer returns this — every Observable
	// is now usable regardless of whether it happens to also implement
	// coord.Object directly (see observableObject in visible_tonight.go).
	ErrNotCoordObject = errors.New("object does not implement coord.Object required for ranking")
	// ErrStepNotPositive indicates a non-positive time step.
	ErrStepNotPositive = errors.New("step must be positive")
	// ErrStepTooLarge indicates a step that risks missing short visibility windows.
	ErrStepTooLarge = errors.New("step exceeds maximum: large steps risk missing short visibility windows")
	// ErrNotObservable indicates the object does not implement Observable.
	ErrNotObservable = errors.New("object does not implement Observable")

	// ErrBracketingViolated indicates f(a) and f(b) have the same sign in root finding.
	ErrBracketingViolated = errors.New("solver: bracketing condition violated: f(a) and f(b) have the same sign")
	// ErrNonFiniteEvaluation indicates an Evaluator returned NaN or ±Inf, or
	// a solver's internal step computation produced a non-finite trial
	// point — both FindRoot and FindExtremum guard against silently
	// converging on (and returning as success) a non-finite result.
	ErrNonFiniteEvaluation = errors.New("solver: evaluator returned a non-finite value")

	// ErrNoConvergence indicates the solver exhausted Solver.MaxIter without
	// bringing the bracket inside Solver.Tolerance.
	//
	// The best estimate found is returned alongside it, so a caller who is
	// content with an approximation can check for this sentinel and keep the
	// value. What is no longer possible is mistaking one for the other: the
	// solver used to return its best effort with a nil error, so a caller
	// doing the ordinary Go thing could not tell a converged root from an
	// iteration budget that ran out. For an interactive display that is
	// defensible; for anything that points a telescope it is not.
	ErrNoConvergence = errors.New("solver: did not converge within MaxIter")
	// ErrEventNotFound indicates no event was found in the search window.
	ErrEventNotFound = errors.New("no event found in search window")

	// ErrUnknownSite indicates NewKnownSite found no entry matching the
	// requested name (checked against every known site's name and
	// aliases, case- and space-insensitive — see KnownSites).
	ErrUnknownSite = errors.New("plan: unknown site name")

	// ErrNoPhysicalRadius indicates AngularDiameter was asked for a body
	// with no known equatorial radius (e.g. an asteroid or comet) — see
	// BodyEquatorialRadius for the covered set.
	ErrNoPhysicalRadius = errors.New("plan: no known physical radius for this body")
	// ErrZeroDistance indicates AngularDiameter was asked to compute a
	// diameter at zero (or negative) topocentric distance, which would
	// otherwise divide by zero.
	ErrZeroDistance = errors.New("plan: distance is zero or negative")

	// ErrUnknownMeteorShower indicates NewMeteorShower found no entry
	// matching the requested name (checked against every shower's Name and
	// Code, case- and space-insensitive — see MeteorShowers).
	ErrUnknownMeteorShower = errors.New("plan: unknown meteor shower name")

	// ErrUnknownPlanetaryMoon indicates NewPlanetaryMoon found no entry
	// matching the requested name (checked case- and space-insensitively
	// against every entry's Name — see PlanetaryMoons).
	ErrUnknownPlanetaryMoon = errors.New("plan: unknown planetary moon name")

	// ErrGeocodeNoResult indicates NewSiteEarthAddress's geocoding query
	// matched no location.
	ErrGeocodeNoResult = errors.New("plan: geocoding found no match for address")
)
