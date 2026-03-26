package coord

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/vector"
)

// ICRS represents a direction and optional distance in the International
// Celestial Reference System (J2000). RA is Right Ascension; Dec is Declination.
type ICRS struct {
	RA   angle.Angle // Right Ascension
	Dec  angle.Angle // Declination
	Dist float64     // Distance in meters (0 if unknown or at infinity)
}

// AltAz represents a direction and optional distance in the local horizontal
// (topocentric) frame. Alt is Altitude; Az is Azimuth (North through East).
type AltAz struct {
	Alt  angle.Angle // Altitude above horizon
	Az   angle.Angle // Azimuth (North through East)
	Dist float64     // Distance in meters
}

// Galactic represents a direction and optional distance in the Galactic
// coordinate system (IAU 1958). L is Galactic longitude; B is Galactic latitude.
type Galactic struct {
	L    angle.Angle // Galactic longitude
	B    angle.Angle // Galactic latitude
	Dist float64     // Distance in meters
}

// Ecliptic represents a direction and optional distance in the Geocentric
// Mean Ecliptic and Equinox (GMEE) coordinate system.
type Ecliptic struct {
	Lon  angle.Angle // Ecliptic longitude
	Lat  angle.Angle // Ecliptic latitude
	Dist float64     // Distance in meters
}

// ── Validation ────────────────────────────────────────────────────────────────

// Validate checks if the coordinate components are finite and within range.
func (c ICRS) Validate() error { return validateLat(c.Dec) }

// Validate checks if the coordinate components are finite and within range.
func (c AltAz) Validate() error { return validateLat(c.Alt) }

// Validate checks if the coordinate components are finite and within range.
func (c Galactic) Validate() error { return validateLat(c.B) }

// Validate checks if the coordinate components are finite and within range.
func (c Ecliptic) Validate() error { return validateLat(c.Lat) }

func validateLat(lat angle.Angle) error {
	d := lat.Degrees()
	if math.IsNaN(d) || math.IsInf(d, 0) {
		return errors.New("coordinate component must be finite")
	}
	if d < -90 || d > 90 {
		return fmt.Errorf("latitude/altitude out of range: %g deg", d)
	}
	return nil
}

// ── ToUnitVector ─────────────────────────────────────────────────────────────

// ToUnitVector converts the direction to a unit Cartesian vector in the
// frame's standard orientation.
func (c ICRS) ToUnitVector() vector.Vec3 {
	return vector.FromSpherical(c.RA.Radians(), c.Dec.Radians())
}

// ToUnitVector converts the direction to a unit Cartesian vector in the
// frame's standard orientation (X=North, Y=East, Z=Up).
func (c AltAz) ToUnitVector() vector.Vec3 {
	alt := c.Alt.Radians()
	az := c.Az.Radians()
	cosAlt := math.Cos(alt)
	return vector.V3(cosAlt*math.Cos(az), cosAlt*math.Sin(az), math.Sin(alt))
}

// ToUnitVector converts the direction to a unit Cartesian vector in the
// frame's standard orientation.
func (c Galactic) ToUnitVector() vector.Vec3 {
	return vector.FromSpherical(c.L.Radians(), c.B.Radians())
}

// ToUnitVector converts the direction to a unit Cartesian vector in the
// frame's standard orientation.
func (c Ecliptic) ToUnitVector() vector.Vec3 {
	return vector.FromSpherical(c.Lon.Radians(), c.Lat.Radians())
}

// ── FromUnitVector ────────────────────────────────────────────────────────────

// ICRSFromUnitVector converts a unit Cartesian vector back to an ICRS coordinate.
func ICRSFromUnitVector(v vector.Vec3) ICRS {
	lon, lat := v.ToSpherical()
	return ICRS{RA: angle.Rad(lon), Dec: angle.Rad(lat)}
}

// GalacticFromUnitVector converts a unit Cartesian vector to a Galactic coordinate.
func GalacticFromUnitVector(v vector.Vec3) Galactic {
	lon, lat := v.ToSpherical()
	return Galactic{L: angle.Rad(lon), B: angle.Rad(lat)}
}

// EclipticFromUnitVector converts a unit Cartesian vector to an Ecliptic coordinate.
func EclipticFromUnitVector(v vector.Vec3) Ecliptic {
	lon, lat := v.ToSpherical()
	return Ecliptic{Lon: angle.Rad(lon), Lat: angle.Rad(lat)}
}

// ── Equality ──────────────────────────────────────────────────────────────────

const coordTol = 1e-12 // radians (~0.2 microarcseconds)

// Equal reports whether c and other point to within coordTol radians of each other.
func (c ICRS) Equal(other ICRS) bool {
	return math.Abs(c.RA.Radians()-other.RA.Radians()) < coordTol &&
		math.Abs(c.Dec.Radians()-other.Dec.Radians()) < coordTol
}

// Equal reports whether c and other point to within coordTol radians of each other.
func (c Galactic) Equal(other Galactic) bool {
	return math.Abs(c.L.Radians()-other.L.Radians()) < coordTol &&
		math.Abs(c.B.Radians()-other.B.Radians()) < coordTol
}

// Equal reports whether c and other point to within coordTol radians of each other.
func (c Ecliptic) Equal(other Ecliptic) bool {
	return math.Abs(c.Lon.Radians()-other.Lon.Radians()) < coordTol &&
		math.Abs(c.Lat.Radians()-other.Lat.Radians()) < coordTol
}

// Equal reports whether c and other point to within coordTol radians of each other.
func (c AltAz) Equal(other AltAz) bool {
	return math.Abs(c.Alt.Radians()-other.Alt.Radians()) < coordTol &&
		math.Abs(c.Az.Radians()-other.Az.Radians()) < coordTol
}

// ── Formatting ────────────────────────────────────────────────────────────────

func (c ICRS) String() string {
	return fmt.Sprintf("ICRS RA=%s Dec=%s", c.RA.Wrap360().HMSString(2), c.Dec.DMSString(2))
}

func (c AltAz) String() string {
	return fmt.Sprintf("AltAz Alt=%s Az=%s", c.Alt.DMSString(2), c.Az.Wrap360().DMSString(2))
}

func (c Galactic) String() string {
	return fmt.Sprintf("Galactic L=%s B=%s", c.L.Wrap360().DMSString(2), c.B.DMSString(2))
}

func (c Ecliptic) String() string {
	return fmt.Sprintf("Ecliptic Lon=%s Lat=%s", c.Lon.Wrap360().DMSString(2), c.Lat.DMSString(2))
}
