package plan

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/time"
)

// Sentinel errors for observation planning.
var (
	// ErrInvalidHorizon is returned when the horizon is outside the valid range.
	ErrInvalidHorizon = errors.New("horizon must be between -90 and 90 degrees")

	// ErrInvalidHorizonHeight is returned for a height above the horizon
	// ([WithHorizonHeight]) that is negative or not a finite length.
	ErrInvalidHorizonHeight = errors.New("plan: height above the horizon must be a finite, non-negative length")

	// ErrNoPrimaryTarget indicates an event spec missing its primary target.
	ErrNoPrimaryTarget = errors.New("event spec must contain a primary target")
	// ErrNoObserverLocation indicates visibility events require a geodetic location.
	ErrNoObserverLocation = errors.New("visibility events require an observer geodetic location")
	// ErrUpperLimbNeedsBody indicates an EventSpec with UpperLimb whose target
	// has no distance to take a semi-diameter from.
	ErrUpperLimbNeedsBody = errors.New("an upper-limb event needs a moving body with a known radius")
	// ErrNoSecondaryTarget indicates a geometry event requires a secondary target.
	ErrNoSecondaryTarget = errors.New("geometry requires a secondary target")
	// ErrReversedInterval is returned for a search interval whose end is before
	// its start.
	ErrReversedInterval = errors.New("plan: interval ends before it starts")
	// ErrNonFiniteInterval is returned for a search interval whose start or
	// end is a NaN or infinite time.
	ErrNonFiniteInterval = errors.New("plan: interval start or end is not a finite time")
	// ErrFamilyNotImpl indicates an event solver for the given family is not implemented.
	ErrFamilyNotImpl = errors.New("event solver for family is not implemented")
	// ErrOppositionNeedsSun indicates an Oppositions call where neither body
	// is the Sun, or both are. An opposition is of a body to the Sun.
	ErrOppositionNeedsSun = errors.New("plan: an opposition is of a body other than the Sun, to the Sun")
	// ErrOppositionScan indicates an opposition search that could not bound
	// its next step: a target whose ecliptic longitude has no bounded rate,
	// on an orbit through the ecliptic's pole.
	ErrOppositionScan = errors.New("plan: opposition search cannot bound its step")
	// ErrUnsupportedGeom indicates an unsupported geometry kind.
	ErrUnsupportedGeom = errors.New("unsupported geometry kind")
	// ErrMoonRequired indicates the illumination solver requires a Moon target.
	ErrMoonRequired = errors.New("illumination solver requires a Moon target")
	// ErrNoSkyDepth indicates a LimitingMagnitudeConstraint with no SkyDepth
	// to ask. There is no default and there cannot be one: a limiting
	// magnitude depends on the detector, the exposure and the observer.
	ErrNoSkyDepth = errors.New("limiting magnitude constraint requires a SkyDepth")

	// ErrNoCoordinates indicates a catalog target that resolves to no
	// position at all, which cannot become a fixed Observable.
	//
	// FromCatalog returns it rather than place the target at the zero value
	// of a missing coordinate, RA 0, Dec 0: a real point in Pisces, where a
	// target would rise, transit, set and schedule like any other.
	ErrNoCoordinates = errors.New("catalog target has no coordinates")

	// ErrNoRadialVelocity indicates a target with no radial velocity to
	// report: a fixed target whose catalog carried none, or a kind with no
	// position model at all. Distinct from a measured RV of exactly zero,
	// which MeasuredRadialVelocity reports as a value with ok true.
	ErrNoRadialVelocity = errors.New("target has no radial velocity")

	// ErrSlewTime marks a slew whose time is not a number of seconds: a NaN
	// slew rate, a NaN slew distance, or a rate so small the time overflows.
	ErrSlewTime = errors.New("plan: slew time is not finite")

	// ErrNonFiniteDuration marks a duration given to plan that is NaN or
	// infinite: a step, a block's Duration or Cadence.MinInterval, or a
	// transition model's setup or penalty.
	ErrNonFiniteDuration = errors.New("plan: duration is not a finite number of seconds")

	// ErrStepNotPositive indicates a non-positive time step.
	ErrStepNotPositive = errors.New("step must be positive")
	// ErrStepTooLarge indicates a step that risks missing short visibility windows.
	ErrStepTooLarge = errors.New("step exceeds maximum: large steps risk missing short visibility windows")
	// ErrNotObservable indicates the object does not implement Observable.
	ErrNotObservable = errors.New("object does not implement Observable")
	// ErrNilSite indicates a nil *Site where one is needed: IsObservable,
	// a constraint's Check, or the Horizon constraint, which reads the site's
	// horizon.
	ErrNilSite = errors.New("plan: site must not be nil")
	// ErrNilTarget indicates a nil Observable where a constraint or
	// IsObservable needs a target's position.
	ErrNilTarget = errors.New("plan: target must not be nil")

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
	// value. A nil error always means a converged root.
	ErrNoConvergence = errors.New("solver: did not converge within MaxIter")
	// ErrEventNotFound indicates no event was found in the search window.
	ErrEventNotFound = errors.New("no event found in search window")

	// ErrNoTwilight indicates the Sun does not cross a twilight threshold
	// within a day, so there is no night of that kind to return: astronomical
	// twilight never ends on a summer night at 51°N, and a polar day or night
	// lasts weeks. NightOf and VisibleTonight return it.
	ErrNoTwilight = errors.New("plan: the Sun does not cross the twilight threshold within a day")
	// ErrUnknownTwilightKind indicates a TwilightKind that TwilightThreshold
	// does not know.
	ErrUnknownTwilightKind = errors.New("plan: unknown twilight kind")

	// ErrUnknownSite indicates NewKnownSite found no entry matching the
	// requested name (checked against every known site's name and
	// aliases, case- and space-insensitive — see KnownSiteNames).
	ErrUnknownSite = errors.New("plan: unknown site name")

	// ErrNoPhysicalRadius indicates AngularDiameter was asked for a body
	// with no known equatorial radius (e.g. an asteroid or comet) — see
	// BodyEquatorialRadius for the covered set.
	ErrNoPhysicalRadius = errors.New("plan: no known physical radius for this body")
	// ErrZeroDistance indicates AngularDiameter was asked to compute a
	// diameter at zero (or negative) topocentric distance, which would
	// otherwise divide by zero.
	ErrZeroDistance = errors.New("plan: distance is zero or negative")

	// ErrSatelliteEclipsed indicates Satellite.ApparentMagnitudeCtx was
	// asked for the magnitude of a satellite in Earth's shadow, which
	// reflects no sunlight and so has none.
	ErrSatelliteEclipsed = errors.New("plan: satellite is in Earth's shadow")

	// ErrUnknownMeteorShower indicates NewMeteorShower found no entry
	// matching the requested name (checked against every shower's Name and
	// Code, case- and space-insensitive — see MeteorShowerNames).
	ErrUnknownMeteorShower = errors.New("plan: unknown meteor shower name")

	// ErrUnknownPlanetaryMoon indicates NewPlanetaryMoon found no entry
	// matching the requested name (checked case- and space-insensitively
	// against every entry's Name — see PlanetaryMoons).
	ErrUnknownPlanetaryMoon = errors.New("plan: unknown planetary moon name")

	// ErrGeocodeNoResult indicates NewSiteFromAddress's geocoding query
	// matched no location.
	ErrGeocodeNoResult = errors.New("plan: geocoding found no match for address")

	// ErrSiteNotOnEarth indicates NewMPCSite was given a real MPC
	// observatory code that has no position on the ground — a space
	// telescope, the geocenter, or a roving-observer placeholder. Distinct
	// from ErrUnknownSite on purpose: the code is valid and the caller's
	// spelling is not the problem.
	ErrSiteNotOnEarth = errors.New("plan: MPC code has no position on Earth")

	// ErrNoMPCObservatories indicates the MPC observatory-code list parsed
	// cleanly and contained no rows, which is what a truncated or
	// redirected fetch looks like — never a real state of the register.
	ErrNoMPCObservatories = errors.New("plan: MPC observatory list is empty")

	// ErrMalformedMPCRow indicates a row of the MPC observatory-code list
	// carried a longitude or parallax constant that is not a number.
	ErrMalformedMPCRow = errors.New("plan: malformed MPC observatory row")
)

// checkInterval refuses a search interval that ends before it starts, or whose
// start or end is not a finite time. An empty one, start equal to end, is
// valid: it has nothing in it.
func checkInterval(start, end time.Time) error {
	// First, because a non-finite time compares false both ways and so is
	// never reversed. FromJD(+Inf) is one too: its JD is NaN (#996).
	if !finiteTime(start) || !finiteTime(end) {
		return fmt.Errorf("%w: [%v, %v]", ErrNonFiniteInterval, start, end)
	}

	if end.Before(start) {
		return fmt.Errorf("%w: [%v, %v]", ErrReversedInterval, start, end)
	}

	return nil
}

// finiteTime reports whether t's Julian date is a finite number.
func finiteTime(t time.Time) bool {
	jd := t.JD()

	return !math.IsNaN(jd) && !math.IsInf(jd, 0)
}
