package coord

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/vector"
)

// Ellipsoid represents a reference ellipsoid for the Earth.
type Ellipsoid struct {
	A float64 // Semi-major axis (meters)
	F float64 // Flattening
}

// WGS84 returns the WGS84 reference ellipsoid.
func WGS84() Ellipsoid {
	return Ellipsoid{
		A: constants.WGS84SemiMajorAxis,
		F: constants.WGS84Flattening,
	}
}

// ── Geodetic Coordinate ──────────────────────────────────────────────────────

// Geodetic represents a point on the Earth using ellipsoidal coordinates.
type Geodetic struct {
	lon    angle.Angle // Longitude
	lat    angle.Angle // Latitude
	height float64     // Height above the ellipsoid (meters)
}

// NewGeodetic creates a new Geodetic coordinate with validation.
// Latitude must be in [-90, 90] degrees. All values must be finite.
func NewGeodetic(lon, lat angle.Angle, height float64) (*Geodetic, error) {
	if math.IsNaN(lon.Radians()) || math.IsInf(lon.Radians(), 0) ||
		math.IsNaN(lat.Radians()) || math.IsInf(lat.Radians(), 0) ||
		math.IsNaN(height) || math.IsInf(height, 0) {
		return nil, errors.New("geodetic coordinates must be finite")
	}

	if lat.Degrees() < -90 || lat.Degrees() > 90 {
		return nil, errors.New("latitude must be between -90 and 90 degrees")
	}

	return &Geodetic{lon: lon, lat: lat, height: height}, nil
}

// NewEarthLocation creates a Geodetic coordinate from latitude, longitude
// (in degrees) and height above the ellipsoid (in meters).
//
// This is a convenience wrapper around [NewGeodetic] that accepts plain
// float64 values in the natural (lat, lon) order used by GPS receivers
// and mapping services (Google Maps, OpenStreetMap, etc.).
//
// Example:
//
//	loc, _ := coord.NewEarthLocation(-23.5505, -46.6333, 760) // São Paulo
func NewEarthLocation(latDeg, lonDeg, heightMeters float64) (*Geodetic, error) {
	return NewGeodetic(angle.Deg(lonDeg), angle.Deg(latDeg), heightMeters)
}

func (g *Geodetic) Lon() angle.Angle {
	return g.lon
}

func (g *Geodetic) Lat() angle.Angle {
	return g.lat
}

func (g *Geodetic) Height() float64 {
	return g.height
}

// ── CoordinateSystem Implementation ──────────────────────────────────────────

func (g *Geodetic) Name() string {
	return "Geodetic"
}

func (g *Geodetic) Validate() error {
	if g.lat.Degrees() < -90 || g.lat.Degrees() > 90 {
		return errors.New("latitude must be between -90 and 90 degrees")
	}

	return nil
}

func (g *Geodetic) ToUnitVector() vector.Vec3 {
	// Represents the unit direction outward from the center of the Earth.
	// We extract it normalized effectively discarding height for the pure unit spherical representation.
	phi := g.lat.Radians()
	lam := g.lon.Radians()

	cosPhi := math.Cos(phi)

	return vector.V3(
		cosPhi*math.Cos(lam),
		cosPhi*math.Sin(lam),
		math.Sin(phi),
	)
}

func (g *Geodetic) FromUnitVector(v vector.Vec3) {
	// Restores the Lon/Lat from a unit direction. Height is zeroed.
	p := math.Hypot(v.X, v.Y)
	lat := math.Atan2(v.Z, p)
	lon := math.Atan2(v.Y, v.X)
	g.lon = angle.Rad(lon).WrapPi()
	g.lat = angle.Rad(lat)
	g.height = 0
}

func (g *Geodetic) Equal(other *Geodetic) bool {
	if other == nil {
		return false
	}

	return math.Abs(g.lon.Radians()-other.lon.Radians()) < 1e-12 &&
		math.Abs(g.lat.Radians()-other.lat.Radians()) < 1e-12 &&
		math.Abs(g.height-other.height) < 1e-6
}

// ── Transformations ──────────────────────────────────────────────────────────

// ToECEF converts Geodetic coordinates to an ECEF (Earth-Centered, Earth-Fixed)
// Cartesian vector using the given ellipsoid.
func (g Geodetic) ToECEF(e Ellipsoid) vector.Vec3 {
	phi := g.lat.Radians()
	lam := g.lon.Radians()
	h := g.height

	sinPhi := math.Sin(phi)
	cosPhi := math.Cos(phi)
	sinLam := math.Sin(lam)
	cosLam := math.Cos(lam)

	// Eccentricity squared: e2 = 2f - f^2
	e2 := 2*e.F - e.F*e.F
	// Prime vertical radius of curvature
	n := e.A / math.Sqrt(1-e2*sinPhi*sinPhi)

	x := (n + h) * cosPhi * cosLam
	y := (n + h) * cosPhi * sinLam
	z := (n*(1-e2) + h) * sinPhi

	return vector.V3(x, y, z)
}

// FromECEF converts an ECEF Cartesian vector to Geodetic coordinates using
// the given ellipsoid. It uses the Bowring (1976) algorithm.
func FromECEF(v vector.Vec3, e Ellipsoid) (*Geodetic, error) {
	x, y, z := v.X, v.Y, v.Z
	if math.IsNaN(x) || math.IsInf(x, 0) ||
		math.IsNaN(y) || math.IsInf(y, 0) ||
		math.IsNaN(z) || math.IsInf(z, 0) {
		return nil, errors.New("ECEF coordinates must be finite")
	}

	a := e.A
	f := e.F
	e2 := 2*f - f*f
	b := a * (1 - f)
	ep2 := (a*a - b*b) / (b * b)

	p := math.Hypot(x, y)
	if p == 0 {
		if z == 0 {
			return NewGeodetic(angle.Rad(0), angle.Rad(0), -a)
		}

		lat := math.Pi / 2
		if z < 0 {
			lat = -math.Pi / 2
		}

		return NewGeodetic(angle.Rad(0), angle.Rad(lat), math.Abs(z)-b)
	}

	theta := math.Atan2(z*a, p*b)

	sinTheta := math.Sin(theta)
	cosTheta := math.Cos(theta)

	lon := math.Atan2(y, x)
	lat := math.Atan2(z+ep2*b*sinTheta*sinTheta*sinTheta, p-e2*a*cosTheta*cosTheta*cosTheta)

	sinLat := math.Sin(lat)
	n := a / math.Sqrt(1-e2*sinLat*sinLat)
	h := p/math.Cos(lat) - n

	return NewGeodetic(angle.Rad(lon).WrapPi(), angle.Rad(lat), h)
}

// String returns a DMS representation of the geodetic coordinate.
func (g *Geodetic) String() string {
	return fmt.Sprintf("Lon=%s, Lat=%s, H=%.1fm",
		g.Lon().DMSString(0), g.Lat().DMSString(0), g.Height())
}
