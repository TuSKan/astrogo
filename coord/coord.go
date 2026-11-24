package coord

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// Sentinel errors for coordinate validation.
var (
	ErrNotFinite         = errors.New("coordinate component must be finite")
	ErrLatitudeRange     = errors.New("latitude/altitude out of range")
	ErrPropagationFailed = errors.New("coord: space-motion propagation failed")
	// ErrZeroVector indicates SubPoint was given a zero-length direction
	// vector, which has no well-defined right ascension/declination.
	ErrZeroVector = errors.New("coord: zero-length vector has no direction")
	// ErrTooFewPoints indicates SmallCircle was asked for fewer than 3
	// points, too few to describe a circle.
	ErrTooFewPoints = errors.New("coord: at least 3 points are required")
	// ErrNilCenter indicates SmallCircle was given a nil center.
	ErrNilCenter = errors.New("coord: center must not be nil")
	// ErrInvalidEllipsoid indicates an Ellipsoid no conversion can use: a
	// semi-major axis that is not positive, or a flattening outside [0, 1).
	ErrInvalidEllipsoid = errors.New("coord: invalid ellipsoid")
	// ErrSofaEpv00Failed indicates gofaext.Epv00 returned a failure
	// status — see HeliocentricRVCorrection, the one place in this
	// package that calls it outside of Context construction.
	ErrSofaEpv00Failed = errors.New("coord: sofa epv00 failed")
)

// Object represents any celestial entity that has a predictable position
// on the sky.
type Object interface {
	// ICRS returns the high-precision ICRS coordinates of the object at time t.
	ICRS(t epoch.Time) (ICRS, error)
}

// ObserversLocation carries the minimal terrestrial metadata needed for
// topocentric frames without depending on the full environment.
type ObserversLocation struct {
	lon    angle.Angle
	lat    angle.Angle
	height unit.Length
}

// Astrometric represents a stellar position with kinematics in the ICRS frame.
type Astrometric struct {
	ra       angle.Angle // Right Ascension
	dec      angle.Angle // Declination
	pmRA     angle.Angle // mu_alpha* = dRA/dt * cos(dec), per Julian year
	pmDec    angle.Angle // Proper Motion in Declination
	parallax angle.Angle // Parallax
	rv       unit.Velocity
}

// CIRS is the geocentric place of an object with aberration, light
// deflection, precession and nutation applied — referred to the true equator
// of date, with right ascension measured from the **Celestial Intermediate
// Origin**. It is the CIRS place, which is what
// [Context.AstrometricToCIRS] produces and what
// [Context.CIRSToObserved] consumes.
//
// This is not the "apparent RA" an almanac prints. That one measures right
// ascension from the true equinox, and the two origins are apart by the
// equation of the origins — about 20 arcminutes in 2026, growing by 46
// arcseconds a year. Declination is the same in both. See [TETE], and
// [Context.CIRSToTETE] to convert.
type CIRS struct {
	ra  angle.Angle
	dec angle.Angle
}

// ICRS represents a direction and optional distance in the International Celestial Reference System.
// It can optionally carry stellar kinematics (proper motion, parallax, radial velocity)
// which, when present, are forwarded to SOFA for rigorous space-motion propagation.
type ICRS struct {
	ra       angle.Angle
	dec      angle.Angle
	dist     unit.Length
	pmRA     angle.Angle // mu_alpha* = dRA/dt * cos(dec), per Julian year
	pmDec    angle.Angle // Proper motion in Dec, per Julian year
	parallax angle.Angle // Stellar parallax
	rv       unit.Velocity

	// epoch is the instant the position holds at, for a position with
	// kinematics; zero reads as J2000.0.
	epoch epoch.Time

	// hasKinematics records whether a caller supplied kinematics, as distinct
	// from their happening to be zero.
	//
	// "No proper motion recorded" and "proper motion measured as zero" are
	// different claims about a star, and they convert differently: the first
	// is a catalog position with no motion information, the second asserts
	// the star is genuinely at rest in ICRS. [ICRSToFK4] and [ICRSToFK5] used
	// to tell them apart by testing every kinematic field for zero, which
	// cannot distinguish them and silently answered the first question when
	// asked the second — see #278, where a star declared at rest in ICRS came
	// back from a round trip with 0.6 to 0.9 mas/yr it never had.
	//
	// FK4 has carried this distinction from the start, through [NewFK4] versus
	// [NewFK4WithProperMotion]. This is the same thing for ICRS.
	hasKinematics bool
}

// AltAz represents a direction and optional distance in the local horizontal frame.
type AltAz struct {
	alt  angle.Angle
	az   angle.Angle
	dist unit.Length
}

// Galactic represents a direction and optional distance in the Galactic coordinate system.
type Galactic struct {
	l    angle.Angle
	b    angle.Angle
	dist unit.Length
}

// Ecliptic represents a direction and optional distance in the Geocentric Mean Ecliptic
type Ecliptic struct {
	lon  angle.Angle
	lat  angle.Angle
	dist unit.Length
}

// ── Constructors ──────────────────────────────────────────────────────────────

// NewICRS creates an ICRS sky direction from right ascension and declination.
func NewICRS(ra, dec angle.Angle) ICRS { return ICRS{ra: ra, dec: dec} }

// NewICRSWithKinematics creates an ICRS direction with stellar kinematics attached.
// SOFA uses these to compute rigorous space-motion propagation, annual parallax,
// and aberration coupling internally via Atcoq/Atciq.
//
// epoch is the instant the position holds at, as the catalog states it:
// J2016.0 for a Gaia DR3 row, J2000.0 for SIMBAD or Hipparcos. Every
// transform moves the star from there, so a row passed in as it was published
// lands where the star is. A zero epoch is J2000.0.
//
// pmRA is mu_alpha* = dRA/dt · cos(dec) — the rate the star moves across the
// sky, which is the pmra column Gaia, SIMBAD and Hipparcos all publish and
// what astropy calls pm_ra_cosdec. Pass a catalog's value straight in.
//
// SOFA's own routines want the other one, dRA/dt, which near a pole is huge
// for a star that is barely moving; converting to it is this package's job
// and happens at the one boundary where SOFA is called. Handed to SOFA
// directly, a catalog value would lose a factor of cos(dec), which is 30% at
// δ = 45° and 83% at δ = 80°.
func NewICRSWithKinematics(ra, dec, pmRA, pmDec, parallax angle.Angle, rv unit.Velocity, epoch epoch.Time) ICRS {
	return ICRS{
		ra: ra, dec: dec,
		pmRA: pmRA, pmDec: pmDec,
		parallax: parallax, rv: rv,
		epoch:         epoch,
		hasKinematics: true,
	}
}

// ICRSFromUnitVector is the direction of v, a vector on ICRS axes, with no
// distance and no kinematics.
func ICRSFromUnitVector(v vector.Vec3) ICRS {
	lon, lat := v.ToSpherical()

	return ICRS{ra: angle.Rad(lon), dec: angle.Rad(lat)}
}

// NewAltAz creates a new AltAz coordinate.
func NewAltAz(alt, az angle.Angle) AltAz {
	return AltAz{alt: alt, az: az}
}

// NewGalactic is the direction at galactic latitude b and longitude l.
func NewGalactic(b angle.Latitude, l angle.Longitude) Galactic {
	return Galactic{l: l.Angle, b: b.Angle}
}

// NewEcliptic is the direction at ecliptic latitude lat and longitude lon.
func NewEcliptic(lat angle.Latitude, lon angle.Longitude) Ecliptic {
	return Ecliptic{lon: lon.Angle, lat: lat.Angle}
}

// NewAstrometric creates a new Astrometric coordinate.
func NewAstrometric(ra, dec angle.Angle) Astrometric {
	return Astrometric{ra: ra, dec: dec}
}

// NewCIRS creates a new CIRS coordinate.
func NewCIRS(ra, dec angle.Angle) CIRS {
	return CIRS{ra: ra, dec: dec}
}

// NewObserversLocation is the observer at latitude lat, longitude lon and
// height above the ellipsoid.
func NewObserversLocation(lat angle.Latitude, lon angle.Longitude, height unit.Length) ObserversLocation {
	return ObserversLocation{lon: lon.Angle, lat: lat.Angle, height: height}
}

// ── Accessors ─────────────────────────────────────────────────────────────────

// RA returns the right ascension of the ICRS coordinate.
func (c ICRS) RA() angle.Angle { return c.ra }

// Dec returns the declination of the ICRS coordinate.
func (c ICRS) Dec() angle.Angle { return c.dec }

// Dist returns the distance of the ICRS coordinate, and zero when none was
// set — which most of the ways an ICRS is built leave it.
//
// It is a [unit.Length]: an ephemeris path sets it in astronomical units and
// a satellite one in kilometers, and a caller asks for the unit it wants.
//
// Zero is still ambiguous, and deliberately so — it means "not set" and it
// means "at the observer", and the type cannot tell them apart. Anything that
// must know carries its own flag, the way kinematics do: [NewICRSWithKinematics]
// records them even when they are zero. Anything that must not guess takes
// the distance as its own parameter, the way [GalactocentricFrame.FromICRS]
// does.
func (c ICRS) Dist() unit.Length { return c.dist }

// PmRA returns the proper motion in right ascension — μα* = dRA/dt · cos(dec),
// the on-sky rate every catalog publishes. See [NewICRSWithKinematics].
func (c ICRS) PmRA() angle.Angle { return c.pmRA }

// PmDec returns the proper motion in declination of the ICRS coordinate.
func (c ICRS) PmDec() angle.Angle { return c.pmDec }

// Parallax returns the parallax of the ICRS coordinate.
func (c ICRS) Parallax() angle.Angle { return c.parallax }

// RV returns the radial velocity of the ICRS coordinate.
func (c ICRS) RV() unit.Velocity { return c.rv }

// Epoch returns the instant the position holds at, as
// [NewICRSWithKinematics] was given it. It is zero, read as J2000.0, for a
// position with no kinematics, which holds at every instant alike.
func (c ICRS) Epoch() epoch.Time { return c.epoch }

// IsZero reports whether this ICRS is the zero value (no coordinates set).
func (c ICRS) IsZero() bool { return c.ra == 0 && c.dec == 0 && c.dist == 0 }

// Astrometric returns this ICRS position as an Astrometric catalog entry at
// J2000.0, the epoch SOFA's catalog routines take, carrying any attached
// kinematics (proper motion, parallax, radial velocity). A position at
// another epoch is moved to J2000.0 first.
func (c ICRS) Astrometric() Astrometric {
	c = c.atJ2000()

	return Astrometric{
		ra: c.ra, dec: c.dec,
		pmRA: c.pmRA, pmDec: c.pmDec,
		parallax: c.parallax, rv: c.rv,
	}
}

// finite reports whether x is neither NaN nor infinite.
func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

// Alt returns the altitude of the AltAz coordinate.
func (c AltAz) Alt() angle.Angle { return c.alt }

// Az returns the azimuth of the AltAz coordinate.
func (c AltAz) Az() angle.Angle { return c.az }

// Dist returns the distance of the AltAz coordinate, carried through the
// transform from the ICRS position it came from. See [ICRS.Dist] for what the
// type does and does not settle.
func (c AltAz) Dist() unit.Length { return c.dist }

// compassPoints are the 16 standard compass points, each spanning 22.5°,
// centered on multiples of 22.5° starting from true north (0°).
var compassPoints = [16]string{
	"N", "NNE", "NE", "ENE", "E", "ESE", "SE", "SSE",
	"S", "SSW", "SW", "WSW", "W", "WNW", "NW", "NNW",
}

// CompassDirection returns az as a 16-point compass label (N, NNE, NE,
// ENE, E, ESE, SE, SSE, S, SSW, SW, WSW, W, WNW, NW, NNW) — the everyday
// "which way do I look" complement to the numeric azimuth degrees.
//
// A non-finite azimuth names no direction, so it gets "".
func CompassDirection(az angle.Angle) string {
	if r := az.Radians(); math.IsNaN(r) || math.IsInf(r, 0) {
		return ""
	}

	deg := az.Wrap360().Degrees()
	idx := int(math.Round(deg/22.5)) % 16

	if idx < 0 {
		idx += 16
	}

	return compassPoints[idx]
}

// Compass returns c's azimuth as a 16-point compass label, or "" for a
// non-finite azimuth — see CompassDirection.
func (c AltAz) Compass() string { return CompassDirection(c.az) }

// SetAlt sets the altitude of the AltAz coordinate.
func (c *AltAz) SetAlt(a angle.Angle) { c.alt = a }

// SetAz sets the azimuth of the AltAz coordinate.
func (c *AltAz) SetAz(a angle.Angle) { c.az = a }

// SetDist sets the distance of the AltAz coordinate.
func (c *AltAz) SetDist(d unit.Length) { c.dist = d }

// L returns the longitude of the Galactic coordinate.
func (c Galactic) L() angle.Longitude { return angle.Longitude{Angle: c.l} }

// B returns the latitude of the Galactic coordinate.
func (c Galactic) B() angle.Latitude { return angle.Latitude{Angle: c.b} }

// Dist returns the distance of the Galactic coordinate.
func (c Galactic) Dist() unit.Length { return c.dist }

// SetL sets the longitude of the Galactic coordinate.
func (c *Galactic) SetL(a angle.Angle) { c.l = a }

// SetB sets the latitude of the Galactic coordinate.
func (c *Galactic) SetB(a angle.Angle) { c.b = a }

// SetDist sets the distance of the Galactic coordinate.
func (c *Galactic) SetDist(d unit.Length) { c.dist = d }

// Lon returns the longitude of the Ecliptic coordinate.
func (c Ecliptic) Lon() angle.Longitude { return angle.Longitude{Angle: c.lon} }

// Lat returns the latitude of the Ecliptic coordinate.
func (c Ecliptic) Lat() angle.Latitude { return angle.Latitude{Angle: c.lat} }

// Dist returns the distance of the Ecliptic coordinate.
func (c Ecliptic) Dist() unit.Length { return c.dist }

// SetLon sets the longitude of the Ecliptic coordinate.
func (c *Ecliptic) SetLon(a angle.Angle) { c.lon = a }

// SetLat sets the latitude of the Ecliptic coordinate.
func (c *Ecliptic) SetLat(a angle.Angle) { c.lat = a }

// SetDist sets the distance of the Ecliptic coordinate.
func (c *Ecliptic) SetDist(d unit.Length) { c.dist = d }

// RA returns the right ascension of the Astrometric coordinate.
func (c Astrometric) RA() angle.Angle { return c.ra }

// Dec returns the declination of the Astrometric coordinate.
func (c Astrometric) Dec() angle.Angle { return c.dec }

// PmRA returns the proper motion in right ascension — μα* = dRA/dt · cos(dec),
// the on-sky rate every catalog publishes. See [NewICRSWithKinematics].
func (c Astrometric) PmRA() angle.Angle { return c.pmRA }

// PmDec returns the proper motion in declination of the Astrometric coordinate.
func (c Astrometric) PmDec() angle.Angle { return c.pmDec }

// Parallax returns the parallax of the Astrometric coordinate.
func (c Astrometric) Parallax() angle.Angle { return c.parallax }

// RV returns the radial velocity of the Astrometric coordinate.
func (c Astrometric) RV() unit.Velocity { return c.rv }

// SetRA sets the right ascension of the Astrometric coordinate.
func (c *Astrometric) SetRA(a angle.Angle) { c.ra = a }

// SetDec sets the declination of the Astrometric coordinate.
func (c *Astrometric) SetDec(a angle.Angle) { c.dec = a }

// SetProperMotion sets the proper motion of the Astrometric coordinate.
func (c *Astrometric) SetProperMotion(pmRA, pmDec angle.Angle) { c.pmRA = pmRA; c.pmDec = pmDec }

// SetParallax sets the parallax of the Astrometric coordinate.
func (c *Astrometric) SetParallax(a angle.Angle) { c.parallax = a }

// SetRV sets the radial velocity of the Astrometric coordinate.
func (c *Astrometric) SetRV(v unit.Velocity) { c.rv = v }

// RA returns the right ascension of the CIRS coordinate.
func (c CIRS) RA() angle.Angle { return c.ra }

// Dec returns the declination of the CIRS coordinate.
func (c CIRS) Dec() angle.Angle { return c.dec }

// SetRA sets the right ascension of the CIRS coordinate.
func (c *CIRS) SetRA(a angle.Angle) { c.ra = a }

// SetDec sets the declination of the CIRS coordinate.
func (c *CIRS) SetDec(a angle.Angle) { c.dec = a }

// Lon returns the longitude of the ObserversLocation coordinate.
func (c ObserversLocation) Lon() angle.Longitude { return angle.Longitude{Angle: c.lon} }

// Lat returns the latitude of the ObserversLocation coordinate.
func (c ObserversLocation) Lat() angle.Latitude { return angle.Latitude{Angle: c.lat} }

// Height returns the height of the ObserversLocation coordinate.
func (c ObserversLocation) Height() unit.Length { return c.height }

// SetLon sets the longitude of the ObserversLocation coordinate.
func (c *ObserversLocation) SetLon(a angle.Angle) { c.lon = a }

// SetLat sets the latitude of the ObserversLocation coordinate.
func (c *ObserversLocation) SetLat(a angle.Angle) { c.lat = a }

// SetHeight sets the height of the ObserversLocation coordinate.
func (c *ObserversLocation) SetHeight(h unit.Length) { c.height = h }

// ── Names ─────────────────────────────────────────────────────────────────────

// Name returns the name of the Astrometric coordinate.
func (c Astrometric) Name() string { return "Astrometric" }

// Name returns the name of the CIRS coordinate.
func (c CIRS) Name() string { return "CIRS" }

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
func (c CIRS) Validate() error { return validateLat(c.dec) }

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
func (c CIRS) ToUnitVector() vector.Vec3 {
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
func (c *CIRS) FromUnitVector(v vector.Vec3) {
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
func (c CIRS) Equal(other CIRS) bool {
	return math.Abs(c.ra.Radians()-other.ra.Radians()) < coordTol &&
		math.Abs(c.dec.Radians()-other.dec.Radians()) < coordTol
}

// Equal checks if the coordinate is equal to the other coordinate.
func (c ObserversLocation) Equal(other ObserversLocation) bool {
	return math.Abs(c.lon.Radians()-other.lon.Radians()) < coordTol &&
		math.Abs(c.lat.Radians()-other.lat.Radians()) < coordTol &&
		math.Abs(c.height.Meters()-other.height.Meters()) < coordTol
}

// ── Formatting ────────────────────────────────────────────────────────────────

func (c Astrometric) String() string {
	return fmt.Sprintf("Astrometric RA=%s Dec=%s", c.ra.Wrap360().HMSString(2), c.dec.DMSString(2))
}

// finite reports whether every component c hands SOFA is finite.
func (c Astrometric) finite() bool {
	return finite(c.ra.Radians()) && finite(c.dec.Radians()) &&
		finite(c.pmRA.Radians()) && finite(c.pmDec.Radians()) &&
		finite(c.parallax.Radians()) && finite(c.rv.MetersPerSec())
}

func (c CIRS) String() string {
	return fmt.Sprintf("CIRS RA=%s Dec=%s", c.ra.Wrap360().HMSString(2), c.dec.DMSString(2))
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
	return fmt.Sprintf("ObserversLocation Lon=%s Lat=%s Height=%f m",
		c.lon.Wrap360().DMSString(2), c.lat.DMSString(2), c.height.Meters())
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
// an ICRS position with kinematics from its own [ICRS.Epoch] to to, and the
// result holds at to. If c has no proper motion, parallax, or radial
// velocity, the position is returned unchanged — propagation over any
// interval is a no-op for a motionless point. A zero epoch, c's or to, is
// J2000.0.
//
// This is rigorous relativistic space-motion propagation (good to
// sub-milliarcsecond over the years-to-decades spans catalog cross-matching
// needs) — a different problem from Context.SetTime's O(1) approximate
// short-span Earth-rotation update, which re-derives observer-frame state,
// not a star's own kinematics.
func PropagateEpoch(c ICRS, to epoch.Time) (ICRS, error) {
	fromEpoch, toEpoch := c.epoch, to
	if fromEpoch.IsZero() {
		fromEpoch = epoch.J2000()
	}

	if toEpoch.IsZero() {
		toEpoch = epoch.J2000()
	}

	if fromEpoch.Equal(toEpoch) {
		return c, nil
	}

	ep1a, ep1b := fromEpoch.TT().JDParts()
	ep2a, ep2b := toEpoch.TT().JDParts()

	ra2, dec2, pmr2, pmd2, px2, rv2, status := gofaext.Pmsafe(
		c.RA().Radians(), c.Dec().Radians(),
		dRAdt(c.PmRA(), c.Dec()), c.PmDec().Radians(),
		c.Parallax().Arcseconds(), c.RV().KmPerSec(),
		ep1a, ep1b, ep2a, ep2b,
	)
	if status < 0 {
		return ICRS{}, fmt.Errorf("%w: status %d", ErrPropagationFailed, status)
	}

	// Pmsafe returns dRA/dt at the new declination; store it back as the
	// on-sky rate, against that declination and not the old one.
	out := NewICRSWithKinematics(
		angle.Rad(ra2), angle.Rad(dec2),
		pmRACosDec(pmr2, angle.Rad(dec2)), angle.Rad(pmd2),
		angle.Arcsec(px2), unit.KmPerSec(rv2),
		toEpoch,
	)
	out.dist = c.dist

	return out, nil
}

// atJ2000 is c moved to J2000.0, where SOFA's catalog routines take a star,
// or c itself when it has no kinematics or is already there.
//
// Pmsafe fails only with SOFA's "system error, should not occur"; c is
// returned as it is then, since this has no error to report it through.
func (c ICRS) atJ2000() ICRS {
	if !c.hasKinematics || c.epoch.IsZero() || c.epoch.Equal(epoch.J2000()) {
		return c
	}

	moved, err := PropagateEpoch(c, epoch.J2000())
	if err != nil {
		return c
	}

	return moved
}
