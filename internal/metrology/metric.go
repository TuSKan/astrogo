package metrology

import (
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/vector"
)

// AngleDifference returns the shortest signed difference got - want, wrapped
// into (-180 deg, +180 deg].
//
// Wrapping is not a detail. 359.999999 degrees and 0.000001 degrees are two
// arcseconds apart, and a comparison that subtracts them reports 360 degrees
// — a number so large it looks like a catastrophic failure and so wrong it
// hides the real one. Every angular comparison in a validation suite crosses
// that boundary eventually, because right ascension wraps once per day and
// azimuth wraps once per turn.
//
// The result keeps its sign so callers can measure bias as well as magnitude;
// take Abs when only the size matters.
func AngleDifference(got, want angle.Angle) angle.Angle {
	d := math.Mod(got.Radians()-want.Radians(), 2*math.Pi)

	switch {
	case d > math.Pi:
		d -= 2 * math.Pi
	case d <= -math.Pi:
		d += 2 * math.Pi
	}

	return angle.Rad(d)
}

// AngularSeparation returns the great-circle angle between two directions
// given as (longitude, latitude) pairs.
//
// It is coordinate-system agnostic on purpose: the same spherical geometry
// serves RA/Dec, azimuth/altitude, and galactic l/b. coord.Separation already
// does this for coord.ICRS values, but reaching it from an AltAz comparison
// means constructing an ICRS that is not one — a trick this repository's
// topocentric suite resorted to, with a comment apologising for it.
//
// The atan2-of-cross-over-dot form is used rather than the cosine rule, which
// loses precision for small separations — and small separations are the ones
// a validation suite spends its time on.
func AngularSeparation(lon1, lat1, lon2, lat2 angle.Angle) angle.Angle {
	dLon := lon2.Radians() - lon1.Radians()

	sinLat1, cosLat1 := math.Sincos(lat1.Radians())
	sinLat2, cosLat2 := math.Sincos(lat2.Radians())
	sinDLon, cosDLon := math.Sincos(dLon)

	x := cosLat2 * sinDLon
	y := cosLat1*sinLat2 - sinLat1*cosLat2*cosDLon
	z := sinLat1*sinLat2 + cosLat1*cosLat2*cosDLon

	return angle.Rad(math.Atan2(math.Hypot(x, y), z))
}

// RelativeError returns |got - want| / |want|.
//
// When want is zero the ratio is undefined, and returning an absolute
// difference in its place — the usual shortcut — silently changes the
// quantity being reported partway through a dataset, so a table of "relative
// errors" ends up mixing two different measurements. This returns NaN
// instead, which propagates visibly and which [Stats] rejects rather than
// averages.
//
// Callers comparing quantities that legitimately pass through zero (a
// velocity component, a signed offset) want an absolute metric for the whole
// dataset, not a relative one for part of it.
func RelativeError(got, want float64) float64 {
	if want == 0 {
		return math.NaN()
	}

	return math.Abs(got-want) / math.Abs(want)
}

// VectorDistance returns the Euclidean norm of a - b, in whatever units the
// two vectors carry.
//
// Preferred over comparing components separately: three per-axis errors are
// three numbers whose combination depends on the frame, while the norm is the
// frame-independent size of the disagreement, which is what a contract on a
// position or a velocity should bound.
func VectorDistance(a, b vector.Vec3) float64 {
	return a.Sub(b).Norm()
}
