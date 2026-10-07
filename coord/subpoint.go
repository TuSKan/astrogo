package coord

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/constants"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// SubPoint returns the geodetic point on Earth's reference ellipsoid where a
// body at geocentric position geocentric (GCRS, in AU, as eph.Position and
// MovingBody.GeocentricVec produce throughout this codebase) is observed
// exactly at the zenith, at time t: the foot of the WGS84 ellipsoid normal
// that passes through the body, at height zero.
//
// The position is used as given: for the point where a body is *seen*
// overhead, pass its apparent place, with light time and aberration in it.
// plan.SubsolarPoint and plan.SublunarPoint do.
//
// # Why the position and not only the direction
//
// The ellipsoid normal at a point does not pass through Earth's centre; it
// is tilted from the geocentric radius by the difference between geodetic
// and geocentric latitude, up to 11.5′ at mid-latitudes. So the point whose
// normal is merely parallel to the body's geocentric direction sees the body
// off the zenith by R⊕ times that tilt over the body's distance. For the Sun
// and the planets that is under a milliarcsecond, and the two answers are
// the same. For the Moon, at 60 Earth radii, it is up to about 10″, 0.3 km on
// the ground: SubPoint used only the direction and called the Moon
// "effectively infinite" until #579. A direction given without a meaningful
// length (a unit vector) is taken as a body that many AU away, where the two
// answers agree. A satellite's position works too: the foot of the normal
// through it is its sub-satellite point.
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

	// The foot of the normal through the body: its Earth-fixed position, in
	// meters, through FromECEF, keeping latitude and longitude.
	au := constants.IAU.AstronomicalUnit.Value

	g, err := FromECEF(vector.V3(itrs[0]*au, itrs[1]*au, itrs[2]*au), WGS84())
	if err != nil {
		return nil, fmt.Errorf("coord: subpoint: %w", err)
	}

	foot, err := NewGeodetic(g.Lon(), g.Lat(), 0)
	if err != nil {
		return nil, fmt.Errorf("coord: subpoint: %w", err)
	}

	return foot, nil
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
