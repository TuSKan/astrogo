package coord

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// Ellipsoid represents a reference ellipsoid for the Earth.
type Ellipsoid struct {
	A unit.Length // Semi-major axis
	F float64     // Flattening
}

// WGS84 returns the WGS84 reference ellipsoid.
//
// The semi-major axis comes from the WGS 84 standard's own defining
// parameters; the flattening f is derived (f = 1/(1/f)) rather than
// tabulated by the standard, so constants publishes it in its Derived
// set — see constants.WGS84.InverseFlattening for the value it comes
// from.
func WGS84() Ellipsoid {
	return Ellipsoid{
		A: unit.Meters(constants.WGS84.SemiMajorAxis.Value),
		F: constants.Derived.WGS84Flattening.Value,
	}
}

// ── Geodetic Coordinate ──────────────────────────────────────────────────────

// Geodetic represents a point on the Earth using ellipsoidal coordinates.
type Geodetic struct {
	lon    angle.Angle // Longitude
	lat    angle.Angle // Latitude
	height unit.Length // Height above the ellipsoid
}

// NewGeodetic creates a new Geodetic coordinate with validation.
// Latitude must be in [-90, 90] degrees. All values must be finite.
func NewGeodetic(lon, lat angle.Angle, height unit.Length) (*Geodetic, error) {
	h := height.Meters()
	if math.IsNaN(lon.Radians()) || math.IsInf(lon.Radians(), 0) ||
		math.IsNaN(lat.Radians()) || math.IsInf(lat.Radians(), 0) ||
		math.IsNaN(h) || math.IsInf(h, 0) {
		return nil, fmt.Errorf("geodetic: %w", ErrNotFinite)
	}

	if lat.Degrees() < -90 || lat.Degrees() > 90 {
		return nil, fmt.Errorf("geodetic: %w", ErrLatitudeRange)
	}

	return &Geodetic{lon: lon, lat: lat, height: height}, nil
}

// MustGeodetic creates a Geodetic coordinate, panicking if parameters are out of range.
func MustGeodetic(lon, lat angle.Angle, height unit.Length) *Geodetic {
	g, err := NewGeodetic(lon, lat, height)
	if err != nil {
		panic(err)
	}

	return g
}

// NewEarthLocation creates a Geodetic coordinate from latitude, longitude
// (in degrees) and height above the ellipsoid (in meters).
//
// This is a convenience wrapper around [NewGeodetic] that accepts plain
// float64 values in the natural (lat, lon) order used by GPS receivers
// and mapping services (Google Maps, OpenStreetMap, etc.).
//
// It keeps plain float64 parameters on purpose, where [NewGeodetic] takes
// typed ones: the numbers it exists to accept are copied straight off such a
// service, where they are always degrees and meters, and the parameter names
// say so. Elsewhere in astrogo a length is a [unit.Length].
//
// Example:
//
//	loc, _ := coord.NewEarthLocation(-23.5505, -46.6333, 760) // São Paulo
func NewEarthLocation(latDeg, lonDeg, heightMeters float64) (*Geodetic, error) {
	return NewGeodetic(angle.Deg(lonDeg), angle.Deg(latDeg), unit.Meters(heightMeters))
}

// Lon returns the longitude of the geodetic coordinate.
func (g *Geodetic) Lon() angle.Angle {
	return g.lon
}

// Lat returns the latitude of the geodetic coordinate.
func (g *Geodetic) Lat() angle.Angle {
	return g.lat
}

// Height returns the height above the ellipsoid.
func (g *Geodetic) Height() unit.Length {
	return g.height
}

// ── CoordinateSystem Implementation ──────────────────────────────────────────

// Name returns the name of the coordinate system.
func (g *Geodetic) Name() string {
	return "Geodetic"
}

// Validate checks if the geodetic coordinate is valid.
func (g *Geodetic) Validate() error {
	if g.lat.Degrees() < -90 || g.lat.Degrees() > 90 {
		return fmt.Errorf("geodetic: %w", ErrLatitudeRange)
	}

	return nil
}

// ToUnitVector represents the unit direction outward from the center of the Earth.
func (g *Geodetic) ToUnitVector() vector.Vec3 {
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

// FromUnitVector restores the Lon/Lat from a unit direction. Height is zeroed.
func (g *Geodetic) FromUnitVector(v vector.Vec3) {
	p := math.Hypot(v.X, v.Y)
	lat := math.Atan2(v.Z, p)
	lon := math.Atan2(v.Y, v.X)
	g.lon = angle.Rad(lon).WrapPi()
	g.lat = angle.Rad(lat)
	g.height = 0
}

// Equal reports whether g and other represent the same geodetic coordinate.
func (g *Geodetic) Equal(other *Geodetic) bool {
	if other == nil {
		return false
	}

	return math.Abs(g.lon.Radians()-other.lon.Radians()) < 1e-12 &&
		math.Abs(g.lat.Radians()-other.lat.Radians()) < 1e-12 &&
		(g.height-other.height).Abs() < unit.Meters(1e-6)
}

// ── Transformations ──────────────────────────────────────────────────────────

// ToECEF converts Geodetic coordinates to an ECEF (Earth-Centered, Earth-Fixed)
// Cartesian vector using the given ellipsoid.
func (g Geodetic) ToECEF(e Ellipsoid) vector.Vec3 {
	phi := g.lat.Radians()
	lam := g.lon.Radians()
	h := g.height.Meters()

	sinPhi := math.Sin(phi)
	cosPhi := math.Cos(phi)
	sinLam := math.Sin(lam)
	cosLam := math.Cos(lam)

	// Eccentricity squared: e2 = 2f - f^2
	e2 := 2*e.F - e.F*e.F
	// Prime vertical radius of curvature
	n := e.A.Meters() / math.Sqrt(1-e2*sinPhi*sinPhi)

	x := (n + h) * cosPhi * cosLam
	y := (n + h) * cosPhi * sinLam
	z := (n*(1-e2) + h) * sinPhi

	return vector.V3(x, y, z)
}

// FromECEF converts an ECEF Cartesian vector, in meters, to geodetic
// coordinates on the given ellipsoid, by SOFA's iauGc2gde: Fukushima's (2006)
// method, which holds to a fraction of a micrometer from the ground out past
// the Moon.
//
// It used Bowring's (1976) single step, which is exact enough on the ground
// and degrades with altitude. Measured round trip, worst height error over
// latitude: 1.5 mm at the ISS, 25 cm at GPS orbit, 31 cm at geostationary
// orbit and 42 cm at the Moon's distance, against 2.2e-8 m at geostationary
// orbit here (#526). It mattered once satellite.Altitude came to run here.
//
// A point on the polar axis gets longitude 0. Returns [ErrNotFinite] for a
// non-finite component and [ErrInvalidEllipsoid] for an ellipsoid SOFA
// refuses.
func FromECEF(v vector.Vec3, e Ellipsoid) (*Geodetic, error) {
	x, y, z := v.X, v.Y, v.Z
	if math.IsNaN(x) || math.IsInf(x, 0) ||
		math.IsNaN(y) || math.IsInf(y, 0) ||
		math.IsNaN(z) || math.IsInf(z, 0) {
		return nil, fmt.Errorf("ECEF: %w", ErrNotFinite)
	}

	lon, lat, h, status := gofaext.Gc2gde(e.A.Meters(), e.F, [3]float64{x, y, z})
	if status != 0 {
		return nil, fmt.Errorf("ECEF: %w: a = %g m, f = %g (SOFA status %d)",
			ErrInvalidEllipsoid, e.A.Meters(), e.F, status)
	}

	return NewGeodetic(angle.Rad(lon).WrapPi(), angle.Rad(lat), unit.Meters(h))
}

// String returns a DMS representation of the geodetic coordinate.
func (g *Geodetic) String() string {
	return fmt.Sprintf("Lon=%s, Lat=%s, H=%.1fm",
		g.Lon().DMSString(0), g.Lat().DMSString(0), g.Height())
}
