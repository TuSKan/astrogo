package skybrightness

import (
	"errors"
	"fmt"

	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/time"
)

// Sentinel errors for scene construction. Match with errors.Is.
var (
	// ErrNoObserver is returned when a Scene has no observer location.
	ErrNoObserver = errors.New("skybrightness: scene needs an observer location")

	// ErrNoAtmosphere is returned when a Scene has no atmospheric state.
	// There is no default: an unstated atmosphere is the single largest
	// source of silent error in a sky-brightness prediction, so it must be
	// chosen explicitly, even if the choice is a climatology.
	ErrNoAtmosphere = errors.New("skybrightness: scene needs an atmosphere")

	// ErrNoTime is returned when a Scene has a zero timestamp.
	ErrNoTime = errors.New("skybrightness: scene needs a time")
)

// Scene is the physical state a sky is evaluated against: who is looking,
// when, and through what.
//
// It is explicit and caller-owned rather than hidden inside components,
// so two evaluations differing in aerosol loading or cloud cover differ in
// a value the caller can see and record, not in a component's private
// default. Every component reads the same Scene, which is what keeps the
// Moon, the artificial sky and the natural sky consistent with one another.
//
// A Scene carries no I/O. Atmospheric and cloud state are fetched by a
// provider layer and handed in already resolved, so evaluation is
// deterministic and makes no network calls.
type Scene struct {
	// Observer is the ground location.
	Observer *coord.Geodetic

	// Time is the instant of the observation.
	Time time.GoTime

	// Atmosphere is the atmospheric state: surface conditions, aerosol,
	// clouds, ozone, precipitable water, vertical profile, horizon and
	// provenance. Owned by the atmosphere package, not redefined here.
	Atmosphere *atmosphere.Atmosphere

	// Ephemeris supplies Sun and Moon positions. Optional in Phase 0,
	// required by the Moon and twilight components once they exist.
	Ephemeris core.Provider
}

// Validate reports whether the scene is usable.
func (s *Scene) Validate() error {
	switch {
	case s == nil:
		return ErrNoObserver
	case s.Observer == nil:
		return ErrNoObserver
	case s.Atmosphere == nil:
		return ErrNoAtmosphere
	case s.Time.IsZero():
		return ErrNoTime
	default:
		return nil
	}
}

// String renders the scene for diagnostics.
func (s *Scene) String() string {
	if s == nil || s.Observer == nil {
		return "skybrightness.Scene(empty)"
	}

	return fmt.Sprintf("skybrightness.Scene(%.4f,%.4f @ %s)",
		s.Observer.Lat().Degrees(), s.Observer.Lon().Degrees(), s.Time.Format(time.RFC3339))
}
