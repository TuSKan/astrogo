package plan

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
)

var (
	ErrInvalidHorizon = errors.New("horizon must be between -90 and 90 degrees")
)

// Site represents a physical observing location.
// Sites are immutable by convention.
type Site struct {
	name     string
	location *coord.Geodetic
	horizon  angle.Angle
	timeZone *time.Location
}

// NewSite creates a new observing site with validation.
// name: A human-readable name for the site.
// loc: The geodetic location (longitude, latitude, height).
// horizon: The local horizon limit (e.g., 0 deg for ideal, 20 deg for trees/hills).
// tz: The local time zone (optional, can be nil).
func NewSite(name string, loc *coord.Geodetic, horizon angle.Angle, tz *time.Location) (*Site, error) {
	if horizon.Degrees() < -90 || horizon.Degrees() > 90 {
		return nil, ErrInvalidHorizon
	}

	// Validate geodetic location (latitude bounds check)
	// NewGeodetic already does this, but we ensure the input is valid here.
	// Actually, we trust coord.Geodetic if it was constructed via NewGeodetic.

	return &Site{
		name:     name,
		location: loc,
		horizon:  horizon,
		timeZone: tz,
	}, nil
}

// Name returns the site's human-readable name.
func (s *Site) Name() string { return s.name }

// Location returns the site's geodetic location.
func (s *Site) Location() *coord.Geodetic { return s.location }

// Horizon returns the local horizon elevation limit.
func (s *Site) Horizon() angle.Angle { return s.horizon }

// TimeZone returns the site's local time zone, or UTC if nil.
func (s *Site) TimeZone() *time.Location {
	if s.timeZone == nil {
		return time.LocationUTC
	}
	return s.timeZone
}

// Longitude returns the site's geodetic longitude.
func (s *Site) Longitude() angle.Angle { return s.location.Lon() }

// Latitude returns the site's geodetic latitude.
func (s *Site) Latitude() angle.Angle { return s.location.Lat() }

// HeightMeters returns the site's height above the reference ellipsoid in meters.
func (s *Site) HeightMeters() float64 { return s.location.Height() }

// Atmosphere returns an atmospheric profile adjusted for the site's elevation
// using the ICAO International Standard Atmosphere barometric formula.
// Pressure and temperature are reduced for altitude; humidity, wavelength,
// and the refraction model are inherited from the sea-level standard.
func (s *Site) Atmosphere() atmosphere.Atmosphere {
	return atmosphere.AtAltitude(s.location.Height())
}

// HorizonDip returns the geometric dip angle of the visible horizon at this
// site's elevation. At sea level the dip is zero; at 786 m it is ≈ 0.90°.
func (s *Site) HorizonDip() angle.Angle {
	return atmosphere.HorizonDip(s.location.Height())
}

// RiseSetThreshold returns the standard rise/set altitude threshold for a
// point source (star) at this site, including the geometric horizon dip
// from the site's elevation.
//
// At sea level: 0°. At 786m: −0.82° (the depressed horizon).
func (s *Site) RiseSetThreshold() angle.Angle {
	return angle.Deg(-s.HorizonDip().Degrees())
}

// SunRiseSetThreshold returns the sunrise/sunset altitude threshold.
// The Sun rises when its observed upper limb touches the visible horizon,
// accounting for the solar semi-diameter (16') and geometric horizon dip
// from the observer's elevation.
func (s *Site) SunRiseSetThreshold() angle.Angle {
	const sunSemiDiameter = 0.2667 // degrees, ~16 arcmin
	return angle.Deg(-sunSemiDiameter - s.HorizonDip().Degrees())
}

// MoonRiseSetThreshold returns the rise/set altitude threshold for the Moon.
// With topocentric parallax handled by the Reducer pipeline (via
// GeocentricToObserved), the threshold accounts for the Moon's mean angular
// semidiameter (~15.5') and the geometric horizon dip from elevation.
func (s *Site) MoonRiseSetThreshold() angle.Angle {
	const moonSemiDiameter = 0.2583 // degrees, ~15.5 arcmin (mean)
	return angle.Deg(-moonSemiDiameter - s.HorizonDip().Degrees())
}

// String returns a compact representation of the site.
func (s *Site) String() string {
	return fmt.Sprintf("Site(%s: %s, Hor=%s)", s.name, s.location, s.horizon)
}

// Equal reports whether s and other represent the same observing site
// (same name, location, horizon, and time zone).
func (s *Site) Equal(other *Site) bool {
	if s == nil || other == nil {
		return s == other
	}

	tzEqual := false
	if s.timeZone == nil && other.timeZone == nil {
		tzEqual = true
	} else if s.timeZone != nil && other.timeZone != nil {
		tzEqual = s.timeZone.String() == other.timeZone.String()
	}

	return s.name == other.name &&
		s.location.Lon().Radians() == other.location.Lon().Radians() &&
		s.location.Lat().Radians() == other.location.Lat().Radians() &&
		s.location.Height() == other.location.Height() &&
		s.horizon.Radians() == other.horizon.Radians() &&
		tzEqual
}

// WithHorizon returns a copy of s with the given horizon limit.
func (s *Site) WithHorizon(h angle.Angle) (*Site, error) {
	return NewSite(s.name, s.location, h, s.timeZone)
}

// WithTimeZone returns a copy of s with the given time zone.
func (s *Site) WithTimeZone(tz *time.Location) *Site {
	return &Site{
		name:     s.name,
		location: s.location,
		horizon:  s.horizon,
		timeZone: tz,
	}
}

// LocalSiderealTime returns the Local Apparent Sidereal Time (LAST) at the
// observer's location for the given time.
//
// LAST = GAST + east longitude
//
// It uses the IAU 2006 GAST model (Gst06a).
func (s *Site) LocalSiderealTime(t time.Time) angle.Angle {
	ut1 := t.UT1()
	u1, u2 := ut1.JDParts()
	tt1, tt2 := t.TT().JDParts()
	gast := gofaext.Gst06a(u1, u2, tt1, tt2)
	lst := gast + s.location.Lon().Radians()
	// Normalise to [0, 2π)
	lst = math.Mod(lst, 2*math.Pi)
	if lst < 0 {
		lst += 2 * math.Pi
	}
	return angle.Rad(lst)
}
