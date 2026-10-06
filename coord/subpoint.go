package coord

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// SubPoint returns the geodetic point on Earth's reference ellipsoid where
// a distant body in direction geocentric (a geocentric position vector in
// GCRS, as produced by eph.Position/MovingBody.GeocentricVec throughout this
// codebase) would be observed exactly at the zenith, at time t. Only
// geocentric's direction matters, not its length/units — it need not be a
// unit vector.
//
// The direction is used as given: for the point where a body is *seen*
// overhead, pass its apparent place, with light time and aberration in it.
// plan.SubsolarPoint and plan.SublunarPoint do.
//
// "At the zenith" means the ellipsoid's local normal is parallel to the
// body's direction. By definition, geodetic latitude IS the angle between
// the equatorial plane and the ellipsoid's normal at a point — so the
// sub-point's geodetic latitude equals the body's declination in the
// Earth-fixed frame directly, with no further ellipsoidal correction, and its
// longitude is the Earth-fixed frame's right-ascension-like angle. This
// differs from the GEOCENTRIC sub-point (the same declination reinterpreted
// as geocentric latitude) by up to ~11.5′ at mid-latitudes — the well-known
// geodetic-vs-geocentric latitude discrepancy under WGS84 flattening.
//
// # The rotation into the Earth-fixed frame
//
// GCRS to ITRS is the full IAU 2006/2000A celestial-to-terrestrial matrix,
// SOFA's C2t06a: frame bias, precession and nutation, then Earth rotation,
// then polar motion, at t's own UT1 and polar motion — the matrix
// [Context] uses. It used to be Earth rotation alone, by GAST, which is the
// right rotation only for a vector already referred to the true equator and
// equinox of date. A GCRS vector is not, so the sub-point was displaced by
// precession and nutation since J2000. For the Sun, measured as the zenith
// distance at the point returned: 15 arcsec at J2000 itself, from nutation,
// 0.38° — 42 km — in October 2026, and growing at the general precession's
// 50 arcsec a year.
//
// # Not for a nearby body
//
// Only the direction is used, so this is the sub-point of a body at
// effectively infinite distance: the Sun, the Moon, a planet. For a
// satellite, whose distance is part of the geometry, the point overhead is
// where the ellipsoid normal passes through the body — [FromECEF] of its
// Earth-fixed position. The two differ in latitude by up to the 11.5′ above
// scaled by R/(R+h): about 10.8′ for the ISS at 420 km.
//
// Returns [ErrZeroVector] for a zero vector, and an error when UT1 cannot be
// had for t, as [time.Time.UT1] reports it.
func SubPoint(geocentric vector.Vec3, t time.Time) (*Geodetic, error) {
	if geocentric.Norm() == 0 {
		return nil, fmt.Errorf("coord: subpoint: %w", ErrZeroVector)
	}

	ut1, err := t.UT1()
	if err != nil {
		return nil, fmt.Errorf("coord: subpoint: %w", err)
	}

	eop := t.EOP()
	tt1, tt2 := t.TT().JDParts()
	u1, u2 := ut1.JDParts()

	rc2t := gofaext.C2t06a(tt1, tt2, u1, u2, eop.XP, eop.YP)
	itrs := gofaext.Rxp(rc2t, [3]float64{geocentric.X, geocentric.Y, geocentric.Z})

	// No ellipsoidal ECEF→geodetic fit, per the doc comment above: the
	// direction's declination and right ascension in ITRS are the geodetic
	// latitude and longitude. FromUnitVector is scale-invariant, so itrs need
	// not be normalized.
	g := &Geodetic{}
	g.FromUnitVector(vector.V3(itrs[0], itrs[1], itrs[2]))

	return g, nil
}

// SmallCircle returns n points forming a spherical small circle of the
// given angular radius around center — e.g. a twilight circle or the
// geometric terminator — as a closed loop of geodetic points, winding counterclockwise
// as seen from outside the sphere (matching increasing longitude at the
// equator). Purely spherical: it treats center's latitude as a direction on
// a sphere, ignoring WGS84 flattening. The resulting ellipsoidal position
// error is at most a few hundred meters — negligible at any scale this
// shape would actually be rendered or reasoned about at.
//
// Returns ErrTooFewPoints if n < 3, or an error if center is nil.
func SmallCircle(center *Geodetic, radius angle.Angle, n int) ([]*Geodetic, error) {
	if n < 3 {
		return nil, fmt.Errorf("coord: smallcircle: n=%d: %w", n, ErrTooFewPoints)
	}

	if center == nil {
		return nil, fmt.Errorf("coord: smallcircle: %w", ErrNilCenter)
	}

	c := center.ToUnitVector()

	// Build an orthonormal basis (u, v) spanning the plane tangent to c.
	// Any consistent choice traces out the same circle — the reference
	// vector is only picked to avoid a near-zero cross product when c is
	// close to it (i.e. close to a pole).
	ref := vector.V3(0, 0, 1)
	if math.Abs(c.Z) > 0.9 {
		ref = vector.V3(1, 0, 0)
	}

	u := c.Cross(ref).Unit()
	v := c.Cross(u) // already unit length: c and u are orthonormal.

	cosR := radius.Cos()
	sinR := radius.Sin()

	points := make([]*Geodetic, n)

	for i := range n {
		az := 2 * math.Pi * float64(i) / float64(n)
		dir := u.MulScalar(math.Cos(az)).Add(v.MulScalar(math.Sin(az)))
		p := c.MulScalar(cosR).Add(dir.MulScalar(sinR))

		g := &Geodetic{}
		g.FromUnitVector(p)
		points[i] = g
	}

	return points, nil
}
