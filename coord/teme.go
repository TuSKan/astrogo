package coord

import (
	"math"

	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/vector"
)

// TEMEToGCRS rotates a geocentric vector from TEME, the frame SGP4 answers in,
// into GCRS at the Context's instant. Length and units pass through, since it
// is a rotation; a velocity is rotated the same way, TEME's own rotation
// against GCRS being the precession rate.
//
// TEME has the true equator of date and the mean equinox: its x-axis is where
// GMST is measured from. The CIO, where the Earth rotation angle is measured
// from, sits GMST − ERA east of it along the same equator, and that angle is a
// polynomial in TT alone — the precession in right ascension, with no
// nutation series in it. So TEME is the CIRS frame of this Context turned by
// that angle about the pole, and GCRS is CIRS through the
// celestial-to-intermediate matrix the Context already holds.
//
// # Why not the bias-precession-nutation matrix
//
// The direct route evaluates the IAU 2006/2000A precession-nutation matrix and
// the equation of the equinoxes at the instant, about 85 µs, and that is what
// [github.com/TuSKan/astrogo/ephemeris/satellite.Satellite.Locate] does, having
// no Context. Here the matrix is the Context's, so the rotation costs the
// polynomial and two matrix products.
//
// The matrix is the one [Context.SetTime] keeps across its window, so this
// GCRS carries that window's drift, as every other place this Context computes
// does. A look angle carries none of it: [Context.GeocentricToObserved] turns
// GCRS back to Earth-fixed through the same matrix, so the two cancel and a
// TEME vector reaches the horizon through GMST and polar motion alone, as it
// should (#812).
func (ctx *Context) TEMEToGCRS(v vector.Vec3) vector.Vec3 {
	tt1, tt2 := ctx.t.TT().JDParts()

	// The CIO is g east of the mean equinox, so a CIRS right ascension is the
	// TEME one less g: the vector turned by −g about the pole.
	g := gofaext.MeanEquinoxFromCIO(tt1, tt2)
	s, c := math.Sincos(g)

	x := c*v.X + s*v.Y
	y := -s*v.X + c*v.Y

	// CIRS → GCRS by the transpose of the celestial-to-intermediate matrix.
	m := &ctx.rc2i

	return vector.Vec3{
		X: m[0][0]*x + m[1][0]*y + m[2][0]*v.Z,
		Y: m[0][1]*x + m[1][1]*y + m[2][1]*v.Z,
		Z: m[0][2]*x + m[1][2]*y + m[2][2]*v.Z,
	}
}
