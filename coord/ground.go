package coord

import (
	"errors"
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
)

// ErrNilGeodetic is returned when a ground calculation is given a nil
// location.
var ErrNilGeodetic = errors.New("coord: geodetic location must not be nil")

// meanEarthRadiusM is the IUGG mean radius R_1 = (2a + b)/3 for WGS84, in
// metres. It is the radius that minimises the error of a spherical
// approximation to the ellipsoid over all latitudes.
const meanEarthRadiusM = 6371008.8

// GroundDistance returns the great-circle distance along the Earth's
// surface between two geodetic locations, in metres.
//
// This is a spherical calculation on the IUGG mean radius, not a geodesic
// on the ellipsoid. The difference reaches roughly 0.3 per cent at
// mid-latitudes, which is well below the uncertainty of anything that
// consumes it here — an artificial-light propagation model's own source
// inventory and aerosol state are uncertain at the tens-of-per-cent level.
// A caller needing survey-grade distance wants Vincenty or Karney, not
// this.
//
// The haversine form is used rather than the spherical law of cosines
// because the latter loses precision for short distances, where
// cos(d/R) approaches 1 — and short distances are exactly the case for a
// nearby light source.
func GroundDistance(a, b *Geodetic) (float64, error) {
	if a == nil || b == nil {
		return 0, ErrNilGeodetic
	}

	lat1 := a.Lat().Radians()
	lat2 := b.Lat().Radians()
	dLat := lat2 - lat1
	dLon := b.Lon().Radians() - a.Lon().Radians()

	sinLat := math.Sin(dLat / 2)
	sinLon := math.Sin(dLon / 2)

	h := sinLat*sinLat + math.Cos(lat1)*math.Cos(lat2)*sinLon*sinLon

	return 2 * meanEarthRadiusM * math.Asin(math.Sqrt(math.Min(1, h))), nil
}

// InitialBearing returns the initial great-circle bearing from a to b,
// measured clockwise from true north and normalised to [0, 360).
//
// It is the *initial* bearing: along a great circle the bearing changes
// continuously, so this is the direction to set off in, not a constant
// heading. For the short paths artificial-light propagation deals with,
// the difference from the final bearing is small, but it is not zero and
// the name says which one this is.
func InitialBearing(a, b *Geodetic) (angle.Angle, error) {
	if a == nil || b == nil {
		return angle.Zero(), ErrNilGeodetic
	}

	lat1 := a.Lat().Radians()
	lat2 := b.Lat().Radians()
	dLon := b.Lon().Radians() - a.Lon().Radians()

	y := math.Sin(dLon) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(dLon)

	deg := math.Atan2(y, x) * 180 / math.Pi

	return angle.Deg(math.Mod(deg+360, 360)), nil
}

// Offset returns the point reached from a start by travelling distanceM
// metres along a great circle on the initial bearing.
//
// It is the direct problem to [GroundDistance] and [InitialBearing]'s
// inverse one, on the same IUGG mean sphere, so the three are mutually
// consistent: offsetting by a computed distance and bearing returns to the
// original point.
//
//	lat2 = asin(sin(lat1)cos(d) + cos(lat1)sin(d)cos(bearing))
//	lon2 = lon1 + atan2(sin(bearing)sin(d)cos(lat1), cos(d) - sin(lat1)sin(lat2))
//
// with d the angular distance. The returned longitude is wrapped to
// (-180, 180]; height is carried over from the start unchanged, since a
// great-circle offset says nothing about terrain.
func Offset(from *Geodetic, bearing angle.Angle, distanceM float64) (*Geodetic, error) {
	if from == nil {
		return nil, ErrNilGeodetic
	}

	if math.IsNaN(distanceM) || math.IsInf(distanceM, 0) {
		return nil, fmt.Errorf("%w: distance %g m", ErrNilGeodetic, distanceM)
	}

	d := distanceM / meanEarthRadiusM

	lat1 := from.Lat().Radians()
	lon1 := from.Lon().Radians()

	sinLat1, cosLat1 := math.Sincos(lat1)
	sinD, cosD := math.Sincos(d)
	sinB, cosB := math.Sincos(bearing.Radians())

	sinLat2 := sinLat1*cosD + cosLat1*sinD*cosB
	sinLat2 = math.Max(-1, math.Min(1, sinLat2))
	lat2 := math.Asin(sinLat2)

	lon2 := lon1 + math.Atan2(sinB*sinD*cosLat1, cosD-sinLat1*sinLat2)

	return NewGeodetic(angle.Rad(lon2).WrapPi(), angle.Rad(lat2), from.Height())
}
