package sky

import (
	"errors"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/observatory"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/transform"
)

var (
	ErrBelowHorizon = errors.New("object is below the horizon")
)

// Object represents any celestial entity that has a predictable position
// on the sky.
type Object interface {
	// ICRS returns the high-precision ICRS coordinates of the object at time t.
	ICRS(t time.Time) (coord.ICRS, error)
}

// Target represents a fixed celestial object with an optional priority.
type Target struct {
	Name     string
	Coord    coord.ICRS
	Priority float64
}

// ICRS implements Object for a fixed Target.
func (t *Target) ICRS(_ time.Time) (coord.ICRS, error) {
	return t.Coord, nil
}

// NewTarget creates a named celestial target at the given RA/Dec (degrees).
func NewTarget(name string, raDeg, decDeg float64) *Target {
	return &Target{
		Name: name,
		Coord: coord.ICRS{
			RA:  angle.Deg(raDeg),
			Dec: angle.Deg(decDeg),
		},
	}
}

func Separation(a, b coord.ICRS) angle.Angle {
	va := a.ToUnitVector()
	vb := b.ToUnitVector()

	// Robust formula for angle between vectors: atan2(|a x b|, a . b)
	cross := va.Cross(vb)
	dot := va.Dot(vb)

	return angle.Atan2(cross.Norm(), dot)
}

// PositionAngle returns the position angle of target 'to' relative to 'from'.
// The result is in ICRS (equatorial) coordinates, measured North through East.
func PositionAngle(from, to coord.ICRS) angle.Angle {
	dra := to.RA.Sub(from.RA).Radians()
	d1 := from.Dec.Radians()
	d2 := to.Dec.Radians()

	y := math.Sin(dra)
	x := math.Cos(d1)*math.Tan(d2) - math.Sin(d1)*math.Cos(dra)

	return angle.Atan2(y, x).Wrap360()
}

// AltAz returns the observer-centric Altitude and Azimuth for a target.
// It is a convenience wrapper around transform.ICRSToAltAz.
func AltAz(target coord.ICRS, t time.Time, site observatory.Site) (coord.AltAz, error) {
	return transform.ICRSToAltAz(target, t, site.Location())
}

// ZenithDistance returns the zenith distance (90 - Alt) for a given altitude.
func ZenithDistance(alt angle.Angle) angle.Angle {
	return angle.Deg(90).Sub(alt)
}

// Airmass returns the relative airmass for a given altitude using the
// Kasten & Young (1989) formula.
//
// Assumptions:
//   - Standard sea-level atmosphere.
//   - Neglects curvature effects below ~5-10 degrees altitude for v1.
//
// Returns an error if the altitude is below the horizon (alt < 0).
func Airmass(alt angle.Angle) (float64, error) {
	if alt.Degrees() < 0 {
		return 0, ErrBelowHorizon
	}

	// Kasten & Young (1989) formula:
	// am = 1 / (sin(alt) + 0.50572 * (6.07995 + alt_deg)^-1.6364)
	// Note: SOFA uses 96.07995 - zd_deg, which is exactly 6.07995 + alt_deg.
	altDeg := alt.Degrees()
	sinAlt := alt.Sin()

	am := 1.0 / (sinAlt + 0.50572*math.Pow(6.07995+altDeg, -1.6364))
	return am, nil
}
