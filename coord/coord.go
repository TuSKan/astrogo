package coord

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// Sentinel errors for coordinate validation.
var (
	ErrNotFinite         = errors.New("coordinate component must be finite")
	ErrLatitudeRange     = errors.New("latitude/altitude out of range")
	ErrPropagationFailed = errors.New("coord: space-motion propagation failed")
)

// Object represents any celestial entity that has a predictable position
// on the sky.
type Object interface {
	// ICRS returns the high-precision ICRS coordinates of the object at time t.
	ICRS(t time.Time) (ICRS, error)
}

// ObserversLocation carries the minimal terrestrial metadata needed for
// topocentric frames without depending on the full environment.
type ObserversLocation struct {
	lon    angle.Angle
	lat    angle.Angle
	height float64 // Meters
}

// Astrometric represents a stellar position with kinematics in the ICRS frame.
type Astrometric struct {
	ra       angle.Angle // Right Ascension
	dec      angle.Angle // Declination
	pmRA     angle.Angle // Proper Motion in Right Ascension
	pmDec    angle.Angle // Proper Motion in Declination
	parallax angle.Angle // Parallax
	rv       float64     // Radial Velocity
}

// Apparent represents the true geocentric position of an object
type Apparent struct {
	ra  angle.Angle
	dec angle.Angle
}

// ICRS represents a direction and optional distance in the International Celestial Reference System.
// It can optionally carry stellar kinematics (proper motion, parallax, radial velocity)
// which, when present, are forwarded to SOFA for rigorous space-motion propagation.
type ICRS struct {
	ra       angle.Angle
	dec      angle.Angle
	dist     float64
	pmRA     angle.Angle // Proper motion in RA (dRA/dt × cos δ), per Julian year
	pmDec    angle.Angle // Proper motion in Dec, per Julian year
	parallax angle.Angle // Stellar parallax
	rv       float64     // Radial velocity (km/s)
}

// AltAz represents a direction and optional distance in the local horizontal frame.
type AltAz struct {
	alt  angle.Angle
	az   angle.Angle
	dist float64
}

// Galactic represents a direction and optional distance in the Galactic coordinate system.
type Galactic struct {
	l    angle.Angle
	b    angle.Angle
	dist float64
}

// Ecliptic represents a direction and optional distance in the Geocentric Mean Ecliptic
type Ecliptic struct {
	lon  angle.Angle
	lat  angle.Angle
	dist float64
}

// ── Constructors ──────────────────────────────────────────────────────────────

// NewICRS creates an ICRS sky direction from right ascension and declination.
func NewICRS(ra, dec angle.Angle) ICRS { return ICRS{ra: ra, dec: dec} }

// NewICRSWithKinematics creates an ICRS direction with stellar kinematics attached.
// SOFA uses these to compute rigorous space-motion propagation, annual parallax,
// and aberration coupling internally via Atcoq/Atciq.
func NewICRSWithKinematics(ra, dec, pmRA, pmDec, parallax angle.Angle, rv float64) ICRS {
	return ICRS{ra: ra, dec: dec, pmRA: pmRA, pmDec: pmDec, parallax: parallax, rv: rv}
}

// NewAltAz creates a new AltAz coordinate.
func NewAltAz(alt, az angle.Angle) AltAz {
	return AltAz{alt: alt, az: az}
}

// NewGalactic creates a new Galactic coordinate.
func NewGalactic(l, b angle.Angle) Galactic {
	return Galactic{l: l, b: b}
}

// NewEcliptic creates a new Ecliptic coordinate.
func NewEcliptic(lon, lat angle.Angle) Ecliptic {
	return Ecliptic{lon: lon, lat: lat}
}

// NewAstrometric creates a new Astrometric coordinate.
func NewAstrometric(ra, dec angle.Angle) Astrometric {
	return Astrometric{ra: ra, dec: dec}
}

// NewApparent creates a new Apparent coordinate.
func NewApparent(ra, dec angle.Angle) Apparent {
	return Apparent{ra: ra, dec: dec}
}

// NewObserversLocation creates a new ObserversLocation.
func NewObserversLocation(lon, lat angle.Angle, height float64) ObserversLocation {
	return ObserversLocation{lon: lon, lat: lat, height: height}
}

// ── Accessors ─────────────────────────────────────────────────────────────────

// RA returns the right ascension of the ICRS coordinate.
func (c ICRS) RA() angle.Angle { return c.ra }

// Dec returns the declination of the ICRS coordinate.
func (c ICRS) Dec() angle.Angle { return c.dec }

// Dist returns the distance of the ICRS coordinate.
func (c ICRS) Dist() float64 { return c.dist }

// PmRA returns the proper motion in right ascension of the ICRS coordinate.
func (c ICRS) PmRA() angle.Angle { return c.pmRA }

// PmDec returns the proper motion in declination of the ICRS coordinate.
func (c ICRS) PmDec() angle.Angle { return c.pmDec }

// Parallax returns the parallax of the ICRS coordinate.
func (c ICRS) Parallax() angle.Angle { return c.parallax }

// RV returns the radial velocity of the ICRS coordinate.
func (c ICRS) RV() float64 { return c.rv }

// SetRA sets the right ascension of the ICRS coordinate.
func (c *ICRS) SetRA(a angle.Angle) { c.ra = a }

// SetDec sets the declination of the ICRS coordinate.
func (c *ICRS) SetDec(a angle.Angle) { c.dec = a }

// SetDist sets the distance of the ICRS coordinate.
func (c *ICRS) SetDist(d float64) { c.dist = d }

// SetProperMotion sets the proper motion of the ICRS coordinate.
func (c *ICRS) SetProperMotion(pmRA, pmDec angle.Angle) { c.pmRA = pmRA; c.pmDec = pmDec }

// SetParallax sets the parallax of the ICRS coordinate.
func (c *ICRS) SetParallax(a angle.Angle) { c.parallax = a }

// SetRV sets the radial velocity of the ICRS coordinate.
func (c *ICRS) SetRV(rv float64) { c.rv = rv }

// IsZero reports whether this ICRS is the zero value (no coordinates set).
func (c ICRS) IsZero() bool { return c.ra == 0 && c.dec == 0 && c.dist == 0 }

// Astrometric returns a copy of this ICRS position as an Astrometric catalog entry,
// carrying any attached kinematics (proper motion, parallax, radial velocity).
func (c ICRS) Astrometric() Astrometric {
	return Astrometric{
		ra: c.ra, dec: c.dec,
		pmRA: c.pmRA, pmDec: c.pmDec,
		parallax: c.parallax, rv: c.rv,
	}
}

// Alt returns the altitude of the AltAz coordinate.
func (c AltAz) Alt() angle.Angle { return c.alt }

// Az returns the azimuth of the AltAz coordinate.
func (c AltAz) Az() angle.Angle { return c.az }

// Dist returns the distance of the AltAz coordinate.
func (c AltAz) Dist() float64 { return c.dist }

// SetAlt sets the altitude of the AltAz coordinate.
func (c *AltAz) SetAlt(a angle.Angle) { c.alt = a }

// SetAz sets the azimuth of the AltAz coordinate.
func (c *AltAz) SetAz(a angle.Angle) { c.az = a }

// SetDist sets the distance of the AltAz coordinate.
func (c *AltAz) SetDist(d float64) { c.dist = d }

// L returns the longitude of the Galactic coordinate.
func (c Galactic) L() angle.Angle { return c.l }

// B returns the latitude of the Galactic coordinate.
func (c Galactic) B() angle.Angle { return c.b }

// Dist returns the distance of the Galactic coordinate.
func (c Galactic) Dist() float64 { return c.dist }

// SetL sets the longitude of the Galactic coordinate.
func (c *Galactic) SetL(a angle.Angle) { c.l = a }

// SetB sets the latitude of the Galactic coordinate.
func (c *Galactic) SetB(a angle.Angle) { c.b = a }

// SetDist sets the distance of the Galactic coordinate.
func (c *Galactic) SetDist(d float64) { c.dist = d }

// Lon returns the longitude of the Ecliptic coordinate.
func (c Ecliptic) Lon() angle.Angle { return c.lon }

// Lat returns the latitude of the Ecliptic coordinate.
func (c Ecliptic) Lat() angle.Angle { return c.lat }

// Dist returns the distance of the Ecliptic coordinate.
func (c Ecliptic) Dist() float64 { return c.dist }

// SetLon sets the longitude of the Ecliptic coordinate.
func (c *Ecliptic) SetLon(a angle.Angle) { c.lon = a }

// SetLat sets the latitude of the Ecliptic coordinate.
func (c *Ecliptic) SetLat(a angle.Angle) { c.lat = a }

// SetDist sets the distance of the Ecliptic coordinate.
func (c *Ecliptic) SetDist(d float64) { c.dist = d }

// RA returns the right ascension of the Astrometric coordinate.
func (c Astrometric) RA() angle.Angle { return c.ra }

// Dec returns the declination of the Astrometric coordinate.
func (c Astrometric) Dec() angle.Angle { return c.dec }

// PmRA returns the proper motion in right ascension of the Astrometric coordinate.
func (c Astrometric) PmRA() angle.Angle { return c.pmRA }

// PmDec returns the proper motion in declination of the Astrometric coordinate.
func (c Astrometric) PmDec() angle.Angle { return c.pmDec }

// Parallax returns the parallax of the Astrometric coordinate.
func (c Astrometric) Parallax() angle.Angle { return c.parallax }

// RV returns the radial velocity of the Astrometric coordinate.
func (c Astrometric) RV() float64 { return c.rv }

// SetRA sets the right ascension of the Astrometric coordinate.
func (c *Astrometric) SetRA(a angle.Angle) { c.ra = a }

// SetDec sets the declination of the Astrometric coordinate.
func (c *Astrometric) SetDec(a angle.Angle) { c.dec = a }

// SetProperMotion sets the proper motion of the Astrometric coordinate.
func (c *Astrometric) SetProperMotion(pmRA, pmDec angle.Angle) { c.pmRA = pmRA; c.pmDec = pmDec }

// SetParallax sets the parallax of the Astrometric coordinate.
func (c *Astrometric) SetParallax(a angle.Angle) { c.parallax = a }

// SetRV sets the radial velocity of the Astrometric coordinate.
func (c *Astrometric) SetRV(v float64) { c.rv = v }

// RA returns the right ascension of the Apparent coordinate.
func (c Apparent) RA() angle.Angle { return c.ra }

// Dec returns the declination of the Apparent coordinate.
func (c Apparent) Dec() angle.Angle { return c.dec }

// SetRA sets the right ascension of the Apparent coordinate.
func (c *Apparent) SetRA(a angle.Angle) { c.ra = a }

// SetDec sets the declination of the Apparent coordinate.
func (c *Apparent) SetDec(a angle.Angle) { c.dec = a }

// Lon returns the longitude of the ObserversLocation coordinate.
func (c ObserversLocation) Lon() angle.Angle { return c.lon }

// Lat returns the latitude of the ObserversLocation coordinate.
func (c ObserversLocation) Lat() angle.Angle { return c.lat }

// Height returns the height of the ObserversLocation coordinate.
func (c ObserversLocation) Height() float64 { return c.height }

// SetLon sets the longitude of the ObserversLocation coordinate.
func (c *ObserversLocation) SetLon(a angle.Angle) { c.lon = a }

// SetLat sets the latitude of the ObserversLocation coordinate.
func (c *ObserversLocation) SetLat(a angle.Angle) { c.lat = a }

// SetHeight sets the height of the ObserversLocation coordinate.
func (c *ObserversLocation) SetHeight(h float64) { c.height = h }

// ── Names ─────────────────────────────────────────────────────────────────────

// Name returns the name of the Astrometric coordinate.
func (c Astrometric) Name() string { return "Astrometric" }

// Name returns the name of the Apparent coordinate.
func (c Apparent) Name() string { return "Apparent" }

// Name returns the name of the ICRS coordinate.
func (c ICRS) Name() string { return "ICRS" }

// Name returns the name of the AltAz coordinate.
func (c AltAz) Name() string { return "AltAz" }

// Name returns the name of the Galactic coordinate.
func (c Galactic) Name() string { return "Galactic" }

// Name returns the name of the Ecliptic coordinate.
func (c Ecliptic) Name() string { return "Ecliptic" }

// Name returns the name of the ObserversLocation coordinate.
func (c ObserversLocation) Name() string { return "ObserversLocation" }

// ── Validation ────────────────────────────────────────────────────────────────

// Validate checks if the coordinate is valid.
func (c Astrometric) Validate() error { return validateLat(c.dec) }

// Validate checks if the coordinate is valid.
func (c Apparent) Validate() error { return validateLat(c.dec) }

// Validate checks if the coordinate is valid.
func (c ICRS) Validate() error { return validateLat(c.dec) }

// Validate checks if the coordinate is valid.
func (c AltAz) Validate() error { return validateLat(c.alt) }

// Validate checks if the coordinate is valid.
func (c Galactic) Validate() error { return validateLat(c.b) }

// Validate checks if the coordinate is valid.
func (c Ecliptic) Validate() error { return validateLat(c.lat) }

// Validate checks if the coordinate is valid.
func (c ObserversLocation) Validate() error { return validateLat(c.lat) }

func validateLat(lat angle.Angle) error {
	d := lat.Degrees()
	if math.IsNaN(d) || math.IsInf(d, 0) {
		return ErrNotFinite
	}

	if d < -90 || d > 90 {
		return fmt.Errorf("%w: %g deg", ErrLatitudeRange, d)
	}

	return nil
}

// ── ToUnitVector ─────────────────────────────────────────────────────────────

// ToUnitVector converts the coordinate to a unit vector.
func (c ICRS) ToUnitVector() vector.Vec3 {
	return vector.FromSpherical(c.ra.Radians(), c.dec.Radians())
}

// ToUnitVector converts the coordinate to a unit vector.
func (c AltAz) ToUnitVector() vector.Vec3 {
	alt := c.alt.Radians()
	az := c.az.Radians()
	cosAlt := math.Cos(alt)

	return vector.V3(cosAlt*math.Cos(az), cosAlt*math.Sin(az), math.Sin(alt))
}

// ToUnitVector converts the coordinate to a unit vector.
func (c Galactic) ToUnitVector() vector.Vec3 {
	return vector.FromSpherical(c.l.Radians(), c.b.Radians())
}

// ToUnitVector converts the coordinate to a unit vector.
func (c Ecliptic) ToUnitVector() vector.Vec3 {
	return vector.FromSpherical(c.lon.Radians(), c.lat.Radians())
}

// ToUnitVector converts the coordinate to a unit vector.
func (c Astrometric) ToUnitVector() vector.Vec3 {
	return vector.FromSpherical(c.ra.Radians(), c.dec.Radians())
}

// ToUnitVector converts the coordinate to a unit vector.
func (c Apparent) ToUnitVector() vector.Vec3 {
	return vector.FromSpherical(c.ra.Radians(), c.dec.Radians())
}

// ToUnitVector converts the coordinate to a unit vector.
func (c ObserversLocation) ToUnitVector() vector.Vec3 {
	return vector.FromSpherical(c.lon.Radians(), c.lat.Radians())
}

// ── FromUnitVector ────────────────────────────────────────────────────────────

// FromUnitVector converts the unit vector to the coordinate.
func (c *Astrometric) FromUnitVector(v vector.Vec3) {
	lon, lat := v.ToSpherical()
	c.ra = angle.Rad(lon)
	c.dec = angle.Rad(lat)
}

// FromUnitVector converts the unit vector to the coordinate.
func (c *Apparent) FromUnitVector(v vector.Vec3) {
	lon, lat := v.ToSpherical()
	c.ra = angle.Rad(lon)
	c.dec = angle.Rad(lat)
}

// FromUnitVector converts the unit vector to the coordinate.
func (c *ICRS) FromUnitVector(v vector.Vec3) {
	lon, lat := v.ToSpherical()
	c.ra = angle.Rad(lon)
	c.dec = angle.Rad(lat)
}

// FromUnitVector converts the unit vector to the coordinate.
func (c *Galactic) FromUnitVector(v vector.Vec3) {
	lon, lat := v.ToSpherical()
	c.l = angle.Rad(lon)
	c.b = angle.Rad(lat)
}

// FromUnitVector converts the unit vector to the coordinate.
func (c *Ecliptic) FromUnitVector(v vector.Vec3) {
	lon, lat := v.ToSpherical()
	c.lon = angle.Rad(lon)
	c.lat = angle.Rad(lat)
}

// FromUnitVector converts the unit vector to the coordinate.
func (c *AltAz) FromUnitVector(v vector.Vec3) {
	lon, lat := v.ToSpherical()
	c.az = angle.Rad(lon)
	c.alt = angle.Rad(lat)
}

// FromUnitVector converts the unit vector to the coordinate.
func (c *ObserversLocation) FromUnitVector(v vector.Vec3) {
	lon, lat := v.ToSpherical()
	c.lon = angle.Rad(lon)
	c.lat = angle.Rad(lat)
}

// ── Equality ──────────────────────────────────────────────────────────────────

const coordTol = 1e-12

// Equal checks if the coordinate is equal to the other coordinate.
func (c ICRS) Equal(other ICRS) bool {
	return math.Abs(c.ra.Radians()-other.ra.Radians()) < coordTol &&
		math.Abs(c.dec.Radians()-other.dec.Radians()) < coordTol
}

// Equal checks if the coordinate is equal to the other coordinate.
func (c AltAz) Equal(other AltAz) bool {
	return math.Abs(c.alt.Radians()-other.alt.Radians()) < coordTol &&
		math.Abs(c.az.Radians()-other.az.Radians()) < coordTol
}

// Equal checks if the coordinate is equal to the other coordinate.
func (c Galactic) Equal(other Galactic) bool {
	return math.Abs(c.l.Radians()-other.l.Radians()) < coordTol &&
		math.Abs(c.b.Radians()-other.b.Radians()) < coordTol
}

// Equal checks if the coordinate is equal to the other coordinate.
func (c Ecliptic) Equal(other Ecliptic) bool {
	return math.Abs(c.lon.Radians()-other.lon.Radians()) < coordTol &&
		math.Abs(c.lat.Radians()-other.lat.Radians()) < coordTol
}

// Equal checks if the coordinate is equal to the other coordinate.
func (c Astrometric) Equal(other Astrometric) bool {
	return math.Abs(c.ra.Radians()-other.ra.Radians()) < coordTol &&
		math.Abs(c.dec.Radians()-other.dec.Radians()) < coordTol
}

// Equal checks if the coordinate is equal to the other coordinate.
func (c Apparent) Equal(other Apparent) bool {
	return math.Abs(c.ra.Radians()-other.ra.Radians()) < coordTol &&
		math.Abs(c.dec.Radians()-other.dec.Radians()) < coordTol
}

// Equal checks if the coordinate is equal to the other coordinate.
func (c ObserversLocation) Equal(other ObserversLocation) bool {
	return math.Abs(c.lon.Radians()-other.lon.Radians()) < coordTol &&
		math.Abs(c.lat.Radians()-other.lat.Radians()) < coordTol &&
		math.Abs(c.height-other.height) < coordTol
}

// ── Formatting ────────────────────────────────────────────────────────────────

func (c Astrometric) String() string {
	return fmt.Sprintf("Astrometric RA=%s Dec=%s", c.ra.Wrap360().HMSString(2), c.dec.DMSString(2))
}

func (c Apparent) String() string {
	return fmt.Sprintf("Apparent RA=%s Dec=%s", c.ra.Wrap360().HMSString(2), c.dec.DMSString(2))
}

func (c ICRS) String() string {
	return fmt.Sprintf("ICRS RA=%s Dec=%s", c.ra.Wrap360().HMSString(2), c.dec.DMSString(2))
}

func (c AltAz) String() string {
	return fmt.Sprintf("AltAz Alt=%s Az=%s", c.alt.DMSString(2), c.az.Wrap360().DMSString(2))
}

func (c Galactic) String() string {
	return fmt.Sprintf("Galactic L=%s B=%s", c.l.Wrap360().DMSString(2), c.b.DMSString(2))
}

func (c Ecliptic) String() string {
	return fmt.Sprintf("Ecliptic Lon=%s Lat=%s", c.lon.Wrap360().DMSString(2), c.lat.DMSString(2))
}

func (c ObserversLocation) String() string {
	return fmt.Sprintf("ObserversLocation Lon=%s Lat=%s Height=%f",
		c.lon.Wrap360().DMSString(2), c.lat.DMSString(2), c.height)
}

// ── Geometry Math ────────────────────────────────────────────────────────────

// Separation calculates the angular separation between two ICRS positions
// along a great circle.
func Separation(a, b ICRS) angle.Angle {
	va := a.ToUnitVector()
	vb := b.ToUnitVector()
	cross := va.Cross(vb)
	dot := va.Dot(vb)

	return angle.Atan2(cross.Norm(), dot)
}

// PositionAngle returns the position angle of target 'to' relative to 'from'.
// Measured North through East.
func PositionAngle(from, to ICRS) angle.Angle {
	dra := to.RA().Sub(from.RA()).Radians()
	d1 := from.Dec().Radians()
	d2 := to.Dec().Radians()

	y := math.Sin(dra)
	x := math.Cos(d1)*math.Tan(d2) - math.Sin(d1)*math.Cos(dra)

	return angle.Atan2(y, x).Wrap360()
}

// PropagateEpoch applies rigorous SOFA space-motion propagation (proper
// motion, parallax, light-time/relativistic correction via Pmsafe) to move
// an ICRS position with kinematics from fromEpoch to toEpoch. If c has no
// proper motion, parallax, or radial velocity, the position is returned
// unchanged — propagation over any interval is a no-op for a motionless
// point. A zero fromEpoch or toEpoch (time.Time{}) is treated as
// time.J2000, matching the epoch convention most catalogs cross-matched
// against assume when they don't report one explicitly.
//
// This is rigorous relativistic space-motion propagation (good to
// sub-milliarcsecond over the years-to-decades spans catalog cross-matching
// needs) — a different problem from Context.AtTime's O(1) approximate
// short-span Earth-rotation update, which re-derives observer-frame state,
// not a star's own kinematics.
func PropagateEpoch(c ICRS, fromEpoch, toEpoch time.Time) (ICRS, error) {
	if fromEpoch.IsZero() {
		fromEpoch = time.J2000
	}

	if toEpoch.IsZero() {
		toEpoch = time.J2000
	}

	if fromEpoch.Equal(toEpoch) {
		return c, nil
	}

	ep1a, ep1b := fromEpoch.TT().JDParts()
	ep2a, ep2b := toEpoch.TT().JDParts()

	ra2, dec2, pmr2, pmd2, px2, rv2, status := gofaext.Pmsafe(
		c.RA().Radians(), c.Dec().Radians(),
		c.PmRA().Radians(), c.PmDec().Radians(),
		c.Parallax().Arcseconds(), c.RV(),
		ep1a, ep1b, ep2a, ep2b,
	)
	if status < 0 {
		return ICRS{}, fmt.Errorf("%w: status %d", ErrPropagationFailed, status)
	}

	out := NewICRSWithKinematics(angle.Rad(ra2), angle.Rad(dec2), angle.Rad(pmr2), angle.Rad(pmd2), angle.Arcsec(px2), rv2)
	out.SetDist(c.Dist())

	return out, nil
}
