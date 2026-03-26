package earth

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
	Lon    angle.Angle // Longitude
	Lat    angle.Angle // Latitude
	Height float64     // Height above the ellipsoid (meters)
}

// NewGeodetic creates a new Geodetic coordinate with validation.
// Latitude must be in [-90, 90] degrees. All values must be finite.
func NewGeodetic(lon, lat angle.Angle, height float64) (Geodetic, error) {
	if math.IsNaN(lon.Radians()) || math.IsInf(lon.Radians(), 0) ||
		math.IsNaN(lat.Radians()) || math.IsInf(lat.Radians(), 0) ||
		math.IsNaN(height) || math.IsInf(height, 0) {
		return Geodetic{}, errors.New("geodetic coordinates must be finite")
	}
	if lat.Degrees() < -90 || lat.Degrees() > 90 {
		return Geodetic{}, errors.New("latitude must be between -90 and 90 degrees")
	}
	return Geodetic{Lon: lon, Lat: lat, Height: height}, nil
}

// ── Transformations ──────────────────────────────────────────────────────────

// ToECEF converts Geodetic coordinates to an ECEF (Earth-Centered, Earth-Fixed)
// Cartesian vector using the given ellipsoid.
func (g Geodetic) ToECEF(e Ellipsoid) vector.Vec3 {
	phi := g.Lat.Radians()
	lam := g.Lon.Radians()
	h := g.Height

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
func FromECEF(v vector.Vec3, e Ellipsoid) (Geodetic, error) {
	x, y, z := v.X, v.Y, v.Z
	if math.IsNaN(x) || math.IsInf(x, 0) ||
		math.IsNaN(y) || math.IsInf(y, 0) ||
		math.IsNaN(z) || math.IsInf(z, 0) {
		return Geodetic{}, errors.New("ECEF coordinates must be finite")
	}

	a := e.A
	f := e.F
	e2 := 2*f - f*f
	b := a * (1 - f)
	ep2 := (a*a - b*b) / (b * b)

	p := math.Hypot(x, y)
	theta := math.Atan2(z*a, p*b)

	sinTheta := math.Sin(theta)
	cosTheta := math.Cos(theta)

	lon := math.Atan2(y, x)
	lat := math.Atan2(z+ep2*b*sinTheta*sinTheta*sinTheta, p-e2*a*cosTheta*cosTheta*cosTheta)

	sinLat := math.Sin(lat)
	n := a / math.Sqrt(1-e2*sinLat*sinLat)
	h := p/math.Cos(lat) - n

	return Geodetic{
		Lon:    angle.Rad(lon).WrapPi(),
		Lat:    angle.Rad(lat),
		Height: h,
	}, nil
}

// String returns a DMS representation of the geodetic coordinate.
func (g Geodetic) String() string {
	return fmt.Sprintf("Lon=%s, Lat=%s, H=%.1fm",
		g.Lon.DMSString(0), g.Lat.DMSString(0), g.Height)
}
