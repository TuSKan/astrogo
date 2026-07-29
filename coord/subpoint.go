package coord

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// SubPoint returns the geodetic point on Earth's reference ellipsoid where
// a distant body in direction geocentric (a geocentric position vector in
// any GCRS-aligned inertial frame — ICRS-equatorial, as produced by
// eph.Position/MovingBody.GeocentricVec throughout this codebase) would be
// observed exactly at the zenith, at time t. Only geocentric's direction
// matters, not its length/units — it need not be a unit vector.
//
// This is a DIFFERENT computation from ephemeris/satellite's unexported
// subSatellitePoint (which rotates ECI→ECEF via GAST and then runs a full
// ellipsoidal ECEF→geodetic fit via FromECEF): that definition — where the
// straight line from the body through the geocenter pierces the ellipsoid
// — is correct for a NEARBY body like an orbiting satellite, whose own
// radial distance from the geocenter is part of the geometry.
//
// For an effectively-infinite-distance body (the Sun, Moon, or any
// planet — this function takes only a direction, so it makes no
// distinction), "at the zenith" instead means the ellipsoid's local normal
// is parallel to the body's direction. By definition, geodetic latitude
// IS the angle between the equatorial plane and the ellipsoid's normal at
// a point — so the sub-point's geodetic latitude equals the body's
// declination in the Earth-fixed (ECEF) frame directly, with no further
// ellipsoidal correction, and its longitude is the ECEF frame's
// right-ascension-like angle. This differs from the GEOCENTRIC sub-point
// (the same declination reinterpreted as geocentric latitude) by up to
// ~11.5′ at mid-latitudes — the well-known geodetic-vs-geocentric latitude
// discrepancy under WGS84 flattening.
//
// Do not refactor this function onto subSatellitePoint's ECEF-fit pattern,
// or vice versa — they solve genuinely different geometric problems for
// bodies at genuinely different distance regimes.
func SubPoint(geocentric vector.Vec3, t time.Time) (*Geodetic, error) {
	if geocentric.Norm() == 0 {
		return nil, fmt.Errorf("coord: subpoint: %w", ErrZeroVector)
	}

	gast, err := t.GAST()
	if err != nil {
		return nil, fmt.Errorf("coord: subpoint: gast: %w", err)
	}

	cosG := gast.Cos()
	sinG := gast.Sin()

	// Rotate the GCRS-aligned direction into the Earth-fixed (ECEF) frame
	// — the same rotation subSatellitePoint uses — but, per the doc
	// comment above, what happens to the result next is deliberately
	// different: no ellipsoidal ECEF→geodetic fit, just direct
	// declination/right-ascension extraction via FromUnitVector below
	// (which is scale-invariant, so ecef need not be normalized).
	ecef := vector.V3(
		geocentric.X*cosG+geocentric.Y*sinG,
		-geocentric.X*sinG+geocentric.Y*cosG,
		geocentric.Z,
	)

	g := &Geodetic{}
	g.FromUnitVector(ecef)

	return g, nil
}

// SmallCircle returns n points forming a spherical small circle of the
// given angular radius around center — e.g. a twilight circle or the
// geometric terminator — as a closed loop of geodetic points, winding counterclockwise
// as seen from outside the sphere (matching increasing longitude at the
// equator). Purely spherical: it treats center's latitude as a direction on
// a sphere, ignoring WGS84 flattening. The resulting ellipsoidal position
// error is at most a few hundred metres — negligible at any scale this
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
