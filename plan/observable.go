package plan

import (
	"github.com/TuSKan/astrogo/coord"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// Observable represents anything that can appear on the sky at a given time.
type Observable interface {
	// Name returns the display name.
	Name() string
	// Position returns the ICRS coordinates at the given time.
	Position(t time.Time) (coord.ICRS, error)
	// GetDetails retrieves comprehensive properties at the given context.
	GetDetails(ctx *coord.Context, props ...string) (*TargetDetails, error)
}

// MovingBody is implemented by targets with ephemeris providers
// (planets, asteroids, comets, satellites).
type MovingBody interface {
	Observable
	// Provider returns the ephemeris provider.
	Provider() eph.Provider
	// EphID returns the NAIF integer identifier.
	EphID() eph.ID
	// GeocentricVec returns the geocentric position vector (in AU).
	GeocentricVec(t time.Time) (vector.Vec3, error)
}

// MagnitudeComputer is optionally implemented by targets with photometry.
type MagnitudeComputer interface {
	// ApparentMagnitude returns the apparent magnitude without atmospheric extinction.
	ApparentMagnitude(t time.Time) (float64, error)
	// ApparentMagnitudeCtx returns the apparent magnitude with atmospheric corrections.
	ApparentMagnitudeCtx(t time.Time, ctx *coord.Context) (float64, error)
}

// StaticMagnitude is implemented by targets with a catalog magnitude
// that does not vary with time or observer geometry.
type StaticMagnitude interface {
	StaticMagnitude() (float64, bool)
}

// Compile-time assertions that every concrete target type implements the
// interfaces it's documented (README/CHANGELOG) to implement. Interface
// satisfaction in Go is structural and silent — a method signature drift
// drops a type out of an interface with no compiler error, only a missing
// code path discovered at runtime (this already happened once: MoonSep's
// CheckCtx had the wrong parameter list and silently fell out of
// ConstraintCtx — see the assertions in constraint.go). These turn that
// class of regression into a build failure instead.
var (
	_ Observable = (*Star)(nil)
	_ Observable = (*DeepSkyObject)(nil)
	_ Observable = (*Planet)(nil)
	_ Observable = (*Asteroid)(nil)
	_ Observable = (*Comet)(nil)
	_ Observable = (*Satellite)(nil)
	_ Observable = (*GenericBody)(nil)

	_ MovingBody = (*Planet)(nil)
	_ MovingBody = (*Asteroid)(nil)
	_ MovingBody = (*Comet)(nil)
	_ MovingBody = (*Satellite)(nil)
	_ MovingBody = (*GenericBody)(nil)

	_ MagnitudeComputer = (*Planet)(nil)
	_ MagnitudeComputer = (*Asteroid)(nil)
	_ MagnitudeComputer = (*Comet)(nil)
	_ MagnitudeComputer = (*Satellite)(nil)

	_ StaticMagnitude = (*Star)(nil)
	_ StaticMagnitude = (*DeepSkyObject)(nil)
	_ StaticMagnitude = (*Satellite)(nil)
)
