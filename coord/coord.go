package coord

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// CoordinateSystem is the interface for all coordinate reference systems.
type CoordinateSystem interface {
	fmt.Stringer
	Name() string
	Validate() error
	ToUnitVector() vector.Vec3
	FromUnitVector(v vector.Vec3)
	Equal(other CoordinateSystem) bool
}

// Object represents any celestial entity that has a predictable position
// on the sky.
type Object interface {
	// ICRS returns the high-precision ICRS coordinates of the object at time t.
	ICRS(t time.Time) (*ICRS, error)
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

func NewICRS(ra, dec angle.Angle) *ICRS { return &ICRS{ra: ra, dec: dec} }

// NewICRSWithKinematics creates an ICRS direction with stellar kinematics attached.
// SOFA uses these to compute rigorous space-motion propagation, annual parallax,
// and aberration coupling internally via Atcoq/Atciq.
func NewICRSWithKinematics(ra, dec, pmRA, pmDec, parallax angle.Angle, rv float64) *ICRS {
	return &ICRS{ra: ra, dec: dec, pmRA: pmRA, pmDec: pmDec, parallax: parallax, rv: rv}
}
func NewAltAz(alt, az angle.Angle) *AltAz             { return &AltAz{alt: alt, az: az} }
func NewGalactic(l, b angle.Angle) *Galactic          { return &Galactic{l: l, b: b} }
func NewEcliptic(lon, lat angle.Angle) *Ecliptic      { return &Ecliptic{lon: lon, lat: lat} }
func NewAstrometric(ra, dec angle.Angle) *Astrometric { return &Astrometric{ra: ra, dec: dec} }
func NewApparent(ra, dec angle.Angle) *Apparent       { return &Apparent{ra: ra, dec: dec} }
func NewObserversLocation(lon, lat angle.Angle, height float64) *ObserversLocation {
	return &ObserversLocation{lon: lon, lat: lat, height: height}
}

// ── Accessors ─────────────────────────────────────────────────────────────────

func (c *ICRS) RA() angle.Angle       { return c.ra }
func (c *ICRS) Dec() angle.Angle      { return c.dec }
func (c *ICRS) Dist() float64         { return c.dist }
func (c *ICRS) PmRA() angle.Angle     { return c.pmRA }
func (c *ICRS) PmDec() angle.Angle    { return c.pmDec }
func (c *ICRS) Parallax() angle.Angle { return c.parallax }
func (c *ICRS) RV() float64           { return c.rv }
func (c *ICRS) SetRA(a angle.Angle)   { c.ra = a }
func (c *ICRS) SetDec(a angle.Angle)  { c.dec = a }
func (c *ICRS) SetDist(d float64)     { c.dist = d }

// Astrometric returns a copy of this ICRS position as an Astrometric catalog entry,
// carrying any attached kinematics (proper motion, parallax, radial velocity).
func (c *ICRS) Astrometric() *Astrometric {
	return &Astrometric{
		ra: c.ra, dec: c.dec,
		pmRA: c.pmRA, pmDec: c.pmDec,
		parallax: c.parallax, rv: c.rv,
	}
}

func (c *AltAz) Alt() angle.Angle     { return c.alt }
func (c *AltAz) Az() angle.Angle      { return c.az }
func (c *AltAz) Dist() float64        { return c.dist }
func (c *AltAz) SetAlt(a angle.Angle) { c.alt = a }
func (c *AltAz) SetAz(a angle.Angle)  { c.az = a }
func (c *AltAz) SetDist(d float64)    { c.dist = d }

func (c *Galactic) L() angle.Angle     { return c.l }
func (c *Galactic) B() angle.Angle     { return c.b }
func (c *Galactic) Dist() float64      { return c.dist }
func (c *Galactic) SetL(a angle.Angle) { c.l = a }
func (c *Galactic) SetB(a angle.Angle) { c.b = a }
func (c *Galactic) SetDist(d float64)  { c.dist = d }

func (c *Ecliptic) Lon() angle.Angle     { return c.lon }
func (c *Ecliptic) Lat() angle.Angle     { return c.lat }
func (c *Ecliptic) Dist() float64        { return c.dist }
func (c *Ecliptic) SetLon(a angle.Angle) { c.lon = a }
func (c *Ecliptic) SetLat(a angle.Angle) { c.lat = a }
func (c *Ecliptic) SetDist(d float64)    { c.dist = d }

func (c *Astrometric) RA() angle.Angle                         { return c.ra }
func (c *Astrometric) Dec() angle.Angle                        { return c.dec }
func (c *Astrometric) PmRA() angle.Angle                       { return c.pmRA }
func (c *Astrometric) PmDec() angle.Angle                      { return c.pmDec }
func (c *Astrometric) Parallax() angle.Angle                   { return c.parallax }
func (c *Astrometric) RV() float64                             { return c.rv }
func (c *Astrometric) SetRA(a angle.Angle)                     { c.ra = a }
func (c *Astrometric) SetDec(a angle.Angle)                    { c.dec = a }
func (c *Astrometric) SetProperMotion(pmRA, pmDec angle.Angle) { c.pmRA = pmRA; c.pmDec = pmDec }
func (c *Astrometric) SetParallax(a angle.Angle)               { c.parallax = a }
func (c *Astrometric) SetRV(v float64)                         { c.rv = v }

func (c *Apparent) RA() angle.Angle      { return c.ra }
func (c *Apparent) Dec() angle.Angle     { return c.dec }
func (c *Apparent) SetRA(a angle.Angle)  { c.ra = a }
func (c *Apparent) SetDec(a angle.Angle) { c.dec = a }

func (c *ObserversLocation) Lon() angle.Angle     { return c.lon }
func (c *ObserversLocation) Lat() angle.Angle     { return c.lat }
func (c *ObserversLocation) Height() float64      { return c.height }
func (c *ObserversLocation) SetLon(a angle.Angle) { c.lon = a }
func (c *ObserversLocation) SetLat(a angle.Angle) { c.lat = a }
func (c *ObserversLocation) SetHeight(h float64)  { c.height = h }

// ── Names ─────────────────────────────────────────────────────────────────────

func (c *Astrometric) Name() string       { return "Astrometric" }
func (c *Apparent) Name() string          { return "Apparent" }
func (c *ICRS) Name() string              { return "ICRS" }
func (c *AltAz) Name() string             { return "AltAz" }
func (c *Galactic) Name() string          { return "Galactic" }
func (c *Ecliptic) Name() string          { return "Ecliptic" }
func (c *ObserversLocation) Name() string { return "ObserversLocation" }

// ── Validation ────────────────────────────────────────────────────────────────

func (c *Astrometric) Validate() error       { return validateLat(c.dec) }
func (c *Apparent) Validate() error          { return validateLat(c.dec) }
func (c *ICRS) Validate() error              { return validateLat(c.dec) }
func (c *AltAz) Validate() error             { return validateLat(c.alt) }
func (c *Galactic) Validate() error          { return validateLat(c.b) }
func (c *Ecliptic) Validate() error          { return validateLat(c.lat) }
func (c *ObserversLocation) Validate() error { return validateLat(c.lat) }

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

func (c *ICRS) ToUnitVector() vector.Vec3 {
	return vector.FromSpherical(c.ra.Radians(), c.dec.Radians())
}

func (c *AltAz) ToUnitVector() vector.Vec3 {
	alt := c.alt.Radians()
	az := c.az.Radians()
	cosAlt := math.Cos(alt)
	return vector.V3(cosAlt*math.Cos(az), cosAlt*math.Sin(az), math.Sin(alt))
}

func (c *Galactic) ToUnitVector() vector.Vec3 {
	return vector.FromSpherical(c.l.Radians(), c.b.Radians())
}

func (c *Ecliptic) ToUnitVector() vector.Vec3 {
	return vector.FromSpherical(c.lon.Radians(), c.lat.Radians())
}

func (c *Astrometric) ToUnitVector() vector.Vec3 {
	return vector.FromSpherical(c.ra.Radians(), c.dec.Radians())
}

func (c *Apparent) ToUnitVector() vector.Vec3 {
	return vector.FromSpherical(c.ra.Radians(), c.dec.Radians())
}

func (c *ObserversLocation) ToUnitVector() vector.Vec3 {
	return vector.FromSpherical(c.lon.Radians(), c.lat.Radians())
}

// ── FromUnitVector ────────────────────────────────────────────────────────────

func (c *Astrometric) FromUnitVector(v vector.Vec3) {
	lon, lat := v.ToSpherical()
	c.ra = angle.Rad(lon)
	c.dec = angle.Rad(lat)
}

func (c *Apparent) FromUnitVector(v vector.Vec3) {
	lon, lat := v.ToSpherical()
	c.ra = angle.Rad(lon)
	c.dec = angle.Rad(lat)
}

func (c *ICRS) FromUnitVector(v vector.Vec3) {
	lon, lat := v.ToSpherical()
	c.ra = angle.Rad(lon)
	c.dec = angle.Rad(lat)
}

func (c *Galactic) FromUnitVector(v vector.Vec3) {
	lon, lat := v.ToSpherical()
	c.l = angle.Rad(lon)
	c.b = angle.Rad(lat)
}

func (c *Ecliptic) FromUnitVector(v vector.Vec3) {
	lon, lat := v.ToSpherical()
	c.lon = angle.Rad(lon)
	c.lat = angle.Rad(lat)
}

func (c *AltAz) FromUnitVector(v vector.Vec3) {
	lon, lat := v.ToSpherical()
	c.az = angle.Rad(lon)
	c.alt = angle.Rad(lat)
}

func (c *ObserversLocation) FromUnitVector(v vector.Vec3) {
	lon, lat := v.ToSpherical()
	c.lon = angle.Rad(lon)
	c.lat = angle.Rad(lat)
}

// ── Equality ──────────────────────────────────────────────────────────────────

const coordTol = 1e-12

func (c *Astrometric) Equal(other CoordinateSystem) bool {
	o, ok := other.(*Astrometric)
	if !ok {
		return false
	}
	return math.Abs(c.ra.Radians()-o.ra.Radians()) < coordTol &&
		math.Abs(c.dec.Radians()-o.dec.Radians()) < coordTol
}

func (c *Apparent) Equal(other CoordinateSystem) bool {
	o, ok := other.(*Apparent)
	if !ok {
		return false
	}
	return math.Abs(c.ra.Radians()-o.ra.Radians()) < coordTol &&
		math.Abs(c.dec.Radians()-o.dec.Radians()) < coordTol
}

func (c *ICRS) Equal(other CoordinateSystem) bool {
	o, ok := other.(*ICRS)
	if !ok {
		return false
	}
	return math.Abs(c.ra.Radians()-o.ra.Radians()) < coordTol &&
		math.Abs(c.dec.Radians()-o.dec.Radians()) < coordTol
}

func (c *Galactic) Equal(other CoordinateSystem) bool {
	o, ok := other.(*Galactic)
	if !ok {
		return false
	}
	return math.Abs(c.l.Radians()-o.l.Radians()) < coordTol &&
		math.Abs(c.b.Radians()-o.b.Radians()) < coordTol
}

func (c *Ecliptic) Equal(other CoordinateSystem) bool {
	o, ok := other.(*Ecliptic)
	if !ok {
		return false
	}
	return math.Abs(c.lon.Radians()-o.lon.Radians()) < coordTol &&
		math.Abs(c.lat.Radians()-o.lat.Radians()) < coordTol
}

func (c *AltAz) Equal(other CoordinateSystem) bool {
	o, ok := other.(*AltAz)
	if !ok {
		return false
	}
	return math.Abs(c.alt.Radians()-o.alt.Radians()) < coordTol &&
		math.Abs(c.az.Radians()-o.az.Radians()) < coordTol
}

func (c *ObserversLocation) Equal(other CoordinateSystem) bool {
	o, ok := other.(*ObserversLocation)
	if !ok {
		return false
	}
	return math.Abs(c.lon.Radians()-o.lon.Radians()) < coordTol &&
		math.Abs(c.lat.Radians()-o.lat.Radians()) < coordTol &&
		math.Abs(c.height-o.height) < coordTol
}

// ── Formatting ────────────────────────────────────────────────────────────────

func (c *Astrometric) String() string {
	return fmt.Sprintf("Astrometric RA=%s Dec=%s", c.ra.Wrap360().HMSString(2), c.dec.DMSString(2))
}

func (c *Apparent) String() string {
	return fmt.Sprintf("Apparent RA=%s Dec=%s", c.ra.Wrap360().HMSString(2), c.dec.DMSString(2))
}

func (c *ICRS) String() string {
	return fmt.Sprintf("ICRS RA=%s Dec=%s", c.ra.Wrap360().HMSString(2), c.dec.DMSString(2))
}

func (c *AltAz) String() string {
	return fmt.Sprintf("AltAz Alt=%s Az=%s", c.alt.DMSString(2), c.az.Wrap360().DMSString(2))
}

func (c *Galactic) String() string {
	return fmt.Sprintf("Galactic L=%s B=%s", c.l.Wrap360().DMSString(2), c.b.DMSString(2))
}

func (c *Ecliptic) String() string {
	return fmt.Sprintf("Ecliptic Lon=%s Lat=%s", c.lon.Wrap360().DMSString(2), c.lat.DMSString(2))
}

func (c *ObserversLocation) String() string {
	return fmt.Sprintf("ObserversLocation Lon=%s Lat=%s Height=%f", c.lon.Wrap360().DMSString(2), c.lat.DMSString(2), c.height)
}

// ── Geometry Math ────────────────────────────────────────────────────────────

// Separation calculates the angular separation between two coordinate points
// along a great circle.
func Separation(a, b CoordinateSystem) angle.Angle {
	va := a.ToUnitVector()
	vb := b.ToUnitVector()
	cross := va.Cross(vb)
	dot := va.Dot(vb)
	return angle.Atan2(cross.Norm(), dot)
}

// PositionAngle returns the position angle of target 'to' relative to 'from'.
// Measured North through East. It specifically requires Equatorial points for a North pole reference.
func PositionAngle(from, to *ICRS) angle.Angle {
	dra := to.RA().Sub(from.RA()).Radians()
	d1 := from.Dec().Radians()
	d2 := to.Dec().Radians()

	y := math.Sin(dra)
	x := math.Cos(d1)*math.Tan(d2) - math.Sin(d1)*math.Cos(dra)

	return angle.Atan2(y, x).Wrap360()
}
