package coord

import (
	"fmt"

	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// The heliocentric frame — HCRS — has the ICRS axes with its origin moved from
// the solar system barycentre to the centre of the Sun.
//
// # It is a translation, not a rotation
//
// Every other frame in this package turns the axes and leaves the origin
// alone, so a direction is enough to convert. This one does the opposite, and
// that changes what a caller has to supply: moving the origin changes the
// direction to a nearby object and leaves a distant one alone, so there is
// nothing to convert without a distance. These functions take position
// vectors rather than an [ICRS] for that reason.
//
// # How far the origin moves
//
// Further than most people expect. The barycentre is the mass-weighted centre
// of the whole solar system, and Jupiter alone is a thousandth of the Sun's
// mass at five astronomical units — so the Sun swings around the barycentre by
// up to about 0.009 AU, nearly two solar radii. Sampled quarterly across a
// Jupiter period it runs from 0.0006 AU to 0.0092 AU — 0.14 to 1.98 solar
// radii: sometimes the barycentre is deep inside the Sun, and sometimes the
// Sun is entirely outside the point it orbits.
//
// For a star the difference is small but not always ignorable: that offset
// subtends 9 milliarcseconds at one parsec and 90 microarcseconds at a
// hundred. For a solar system body it is neither — the same offset seen from
// one AU is 1868 arcseconds, half a degree, the width of the Moon.
//
// # Why this lives here
//
// The Sun's barycentric *velocity* was already being derived in this package,
// inside [Context.HeliocentricRVCorrection], from the one SOFA routine that
// makes it available — Epv00 returns Earth's heliocentric and barycentric
// state together, and the Sun's barycentric state is their difference. The
// position is the other half of the same subtraction and was simply never
// taken.

// SunBarycentric returns the Sun's position relative to the solar system
// barycentre at t, in AU, on ICRS axes.
//
// There is no SOFA routine for this directly. Epv00 returns Earth's
// heliocentric and barycentric position, and the Sun's barycentric position is
// what is left when one is taken from the other — the same identity
// [Context.HeliocentricRVCorrection] uses for the velocity.
//
// The epoch is used on the TDB scale, which is what Epv00 is defined for.
func SunBarycentric(t time.Time) (vector.Vec3, error) {
	d1, d2 := t.TDB().JDParts()

	pvh, pvb, status := gofaext.Epv00(d1, d2)
	if status < 0 {
		return vector.Vec3{}, fmt.Errorf("%w: status %d", ErrSofaEpv00Failed, status)
	}

	return vector.V3(
		pvb[0][0]-pvh[0][0],
		pvb[0][1]-pvh[0][1],
		pvb[0][2]-pvh[0][2],
	), nil
}

// BarycentricToHeliocentric moves a position vector's origin from the solar
// system barycentre to the centre of the Sun, at epoch t. Both are in AU on
// ICRS axes.
//
// Named rather than left to the caller because the subtraction has a direction
// and getting it backwards doubles the error instead of removing it — the
// result is wrong by twice the Sun's barycentric offset, which still looks
// like a plausible position.
//
// # For more than one position
//
// This calls [SunBarycentric] every time, and that costs about 45 µs: Epv00
// evaluates a planetary series, not a formula. Converting a list of bodies at
// one epoch should take the Sun's position once and subtract it directly —
// the same "build it per epoch and reuse it" rule [NewContext] follows, for
// the same reason and at a similar cost.
//
//	sun, err := coord.SunBarycentric(t)
//	// ... then v.Sub(sun) per body.
func BarycentricToHeliocentric(v vector.Vec3, t time.Time) (vector.Vec3, error) {
	sun, err := SunBarycentric(t)
	if err != nil {
		return vector.Vec3{}, err
	}

	return v.Sub(sun), nil
}

// HeliocentricToBarycentric is the inverse of [BarycentricToHeliocentric].
func HeliocentricToBarycentric(v vector.Vec3, t time.Time) (vector.Vec3, error) {
	sun, err := SunBarycentric(t)
	if err != nil {
		return vector.Vec3{}, err
	}

	return v.Add(sun), nil
}
