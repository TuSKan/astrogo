package coord

import (
	"github.com/TuSKan/astrogo/vector"
)

// ITRS is the International Terrestrial Reference System: axes fixed in the
// rotating Earth, with z through the pole, x through the Greenwich meridian,
// and origin at the geocentre. It is the frame a station's coordinates are
// published in, the frame a GNSS receiver reports, and the frame a satellite's
// ground track is computed in.
//
// # Why it is a rotation and not a conversion
//
// ICRS and ITRS have the same origin and differ only by orientation, so the
// map between them is a single 3×3 rotation — but not a constant one. It is
// rebuilt from three time-dependent pieces, and each is a different physical
// effect:
//
//   - **Precession-nutation**, which moves the celestial pole against the
//     stars over centuries and decades.
//   - **Earth rotation angle**, the daily spin, which is by far the largest
//     term and is driven by UT1 rather than by a uniform clock — which is why
//     DUT1 matters and why a missing IERS bulletin costs up to 0.9 s of
//     rotation, about 14 arcseconds at the equator.
//   - **Polar motion**, the wander of the rotation axis within the Earth
//     itself, a few tenths of an arcsecond of Chandler and annual wobble.
//
// [Context] already builds and caches that rotation for its own epoch, since
// every horizon transform goes through it. These two methods hand it to a
// caller rather than rebuilding it, so an Earth-fixed position costs a matrix
// multiply and not the ~91 µs of a fresh [NewContext].
//
// # What this is not
//
// Not a geodetic conversion. ITRS is Cartesian metres from the geocentre;
// latitude, longitude and height above the ellipsoid come from
// [FromECEF], and the direction-to-ground-point question is [SubPoint].
//
// Not an observer-relative quantity either: no parallax, aberration or
// refraction is applied here, because none of those belong to a change of
// axes. [Context.GeocentricToObserved] is the path that applies them, and it
// uses this same rotation on the way.

// ICRSToITRS rotates a geocentric vector from the celestial frame into the
// Earth-fixed one at the Context's epoch.
//
// The vector is geocentric — a position measured from the centre of the Earth,
// or a direction, in whatever units the caller is working in. Length and units
// pass through unchanged, since this is a rotation.
//
// It is the same rotation [Context.GeocentricToObserved] applies internally,
// taken from the same cached matrix, so the two cannot disagree.
func (ctx *Context) ICRSToITRS(v vector.Vec3) vector.Vec3 {
	return vector.Vec3{
		X: ctx.mat[0][0]*v.X + ctx.mat[0][1]*v.Y + ctx.mat[0][2]*v.Z,
		Y: ctx.mat[1][0]*v.X + ctx.mat[1][1]*v.Y + ctx.mat[1][2]*v.Z,
		Z: ctx.mat[2][0]*v.X + ctx.mat[2][1]*v.Y + ctx.mat[2][2]*v.Z,
	}
}

// ITRSToICRS rotates an Earth-fixed geocentric vector into the celestial
// frame at the Context's epoch. The inverse of [Context.ICRSToITRS].
//
// The matrix is orthogonal, so the inverse is its transpose and is applied as
// one: no inversion is computed, and a round trip returns the original vector
// to float precision rather than to the accuracy of a solve.
func (ctx *Context) ITRSToICRS(v vector.Vec3) vector.Vec3 {
	return vector.Vec3{
		X: ctx.mat[0][0]*v.X + ctx.mat[1][0]*v.Y + ctx.mat[2][0]*v.Z,
		Y: ctx.mat[0][1]*v.X + ctx.mat[1][1]*v.Y + ctx.mat[2][1]*v.Z,
		Z: ctx.mat[0][2]*v.X + ctx.mat[1][2]*v.Y + ctx.mat[2][2]*v.Z,
	}
}
