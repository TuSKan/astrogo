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
	ErrNilLocation    = errors.New("geodetic location must not be nil")
)

// Site represents a physical observing location.
// Sites are immutable by convention.
type Site struct {
	location *coord.Geodetic
	timeZone *time.Location
	name     string
	horizon  angle.Angle
}

// NewSite creates a new observing site with validation.
// name: A human-readable name for the site.
// loc: The geodetic location (longitude, latitude, height).
// horizon: The local horizon limit (e.g., 0 deg for ideal, 20 deg for trees/hills).
// tz: The local time zone (optional, can be nil).
func NewSite(name string, loc *coord.Geodetic, horizon angle.Angle, tz *time.Location) (*Site, error) {
	if loc == nil {
		return nil, ErrNilLocation
	}

	if horizon.Degrees() < -90 || horizon.Degrees() > 90 {
		return nil, ErrInvalidHorizon
	}

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
// The Sun rises when its geometric center is at:
//
//	alt = −(semi-diameter + standard refraction + horizon dip)
//
// This matches the USNO definition (Explanatory Supplement to the
// Astronomical Almanac, §9.311):
//   - Solar semi-diameter: 16' (0.2667°)
//   - Standard atmospheric refraction at horizon: 34' (0.5667°)
//   - Horizon dip from elevation: 1.76'√h
//
// Total at sea level: −(16' + 34') = −50' = −0.8333°.
func (s *Site) SunRiseSetThreshold() angle.Angle {
	const sunSemiDiameter = 0.2667 // degrees, ~16 arcmin

	const standardRefraction = 0.5667 // degrees, ~34 arcmin

	return angle.Deg(-sunSemiDiameter - standardRefraction - s.HorizonDip().Degrees())
}

// MoonRiseSetThreshold returns the rise/set altitude threshold for the Moon.
// Follows the same convention as SunRiseSetThreshold: the Moon rises when
// its geometric center reaches:
//
//	alt = −(semi-diameter + standard refraction + horizon dip)
//
// The Moon's mean semi-diameter is ~15.5' (varies with parallax, handled
// by the topocentric correction in GeocentricToObserved).
func (s *Site) MoonRiseSetThreshold() angle.Angle {
	const moonSemiDiameter = 0.2583 // degrees, ~15.5 arcmin (mean)

	const standardRefraction = 0.5667 // degrees, ~34 arcmin

	return angle.Deg(-moonSemiDiameter - standardRefraction - s.HorizonDip().Degrees())
}

// String returns a compact representation of the site.
func (s *Site) String() string {
	return fmt.Sprintf("Site(%s: %s, Hor=%s)", s.name, s.location, s.horizon)
}

// Equal reports whether s and other represent the same observing site
// (same name, location, horizon, and time zone).
//
// Coordinates and horizon are compared with a tolerance of 1e-12 radians
// (~0.2 μas) to avoid false negatives from float64 round-trip drift.
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

	const eps = 1e-12 // radians, ~0.2 μas

	return s.name == other.name &&
		math.Abs(s.location.Lon().Radians()-other.location.Lon().Radians()) < eps &&
		math.Abs(s.location.Lat().Radians()-other.location.Lat().Radians()) < eps &&
		math.Abs(s.location.Height()-other.location.Height()) < eps &&
		math.Abs(s.horizon.Radians()-other.horizon.Radians()) < eps &&
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
// Returns an error if IERS EOP data is unavailable for the UT1 conversion.
func (s *Site) LocalSiderealTime(t time.Time) (angle.Angle, error) {
	ut1, err := t.UT1()
	if err != nil {
		return angle.Zero(), fmt.Errorf("LocalSiderealTime: %w", err)
	}

	u1, u2 := ut1.JDParts()
	tt1, tt2 := t.TT().JDParts()
	gast := gofaext.Gst06a(u1, u2, tt1, tt2)
	lst := gast + s.location.Lon().Radians()
	// Normalise to [0, 2π)
	lst = math.Mod(lst, 2*math.Pi)
	if lst < 0 {
		lst += 2 * math.Pi
	}

	return angle.Rad(lst), nil
}
