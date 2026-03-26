package observatory

import (
	"errors"
	"fmt"
	"time"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/earth"
)

var (
	ErrInvalidHorizon = errors.New("horizon must be between -90 and 90 degrees")
)

// Site represents a physical observing location.
// Sites are immutable by convention.
type Site struct {
	name     string
	location earth.Geodetic
	horizon  angle.Angle
	timeZone *time.Location
}

// NewSite creates a new observing site with validation.
// name: A human-readable name for the site.
// loc: The geodetic location (longitude, latitude, height).
// horizon: The local horizon limit (e.g., 0 deg for ideal, 20 deg for trees/hills).
// tz: The local time zone (optional, can be nil).
func NewSite(name string, loc earth.Geodetic, horizon angle.Angle, tz *time.Location) (Site, error) {
	if horizon.Degrees() < -90 || horizon.Degrees() > 90 {
		return Site{}, ErrInvalidHorizon
	}

	// Validate geodetic location (latitude bounds check)
	// NewGeodetic already does this, but we ensure the input is valid here.
	// Actually, we trust earth.Geodetic if it was constructed via NewGeodetic.

	return Site{
		name:     name,
		location: loc,
		horizon:  horizon,
		timeZone: tz,
	}, nil
}

// Name returns the site's human-readable name.
func (s Site) Name() string { return s.name }

// Location returns the site's geodetic location.
func (s Site) Location() earth.Geodetic { return s.location }

// Horizon returns the local horizon elevation limit.
func (s Site) Horizon() angle.Angle { return s.horizon }

// TimeZone returns the site's local time zone, or UTC if nil.
func (s Site) TimeZone() *time.Location {
	if s.timeZone == nil {
		return time.UTC
	}
	return s.timeZone
}

// Longitude returns the site's geodetic longitude.
func (s Site) Longitude() angle.Angle { return s.location.Lon }

// Latitude returns the site's geodetic latitude.
func (s Site) Latitude() angle.Angle { return s.location.Lat }

// HeightMeters returns the site's height above the reference ellipsoid in meters.
func (s Site) HeightMeters() float64 { return s.location.Height }

// String returns a compact representation of the site.
func (s Site) String() string {
	return fmt.Sprintf("Site(%s: %s, Hor=%s)", s.name, s.location, s.horizon)
}
