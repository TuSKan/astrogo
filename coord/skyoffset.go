package coord

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/vector"
)

// SkyOffset is a spherical frame whose origin sits on a chosen target, so that
// positions near it are expressed as offsets from it rather than as absolute
// right ascension and declination.
//
// # Why the obvious arithmetic is wrong
//
// The usual first attempt at "how far is this from my target" is to subtract:
// Δα = α − α₀ and Δδ = δ − δ₀. That is wrong in two ways at once, and both
// grow with declination:
//
//   - A degree of right ascension is a degree of arc only at the equator. At
//     δ = 60° it is half that, and at δ = 89° it is a sixtieth. Quoting Δα as
//     an offset overstates it by 1/cos δ.
//   - Multiplying by cos δ fixes the scale and not the geometry. The lines of
//     constant declination are small circles, not great circles, so a grid
//     built from (Δα·cos δ, Δδ) is sheared — enough to matter across a
//     wide-field detector, and badly wrong anywhere near a pole.
//
// A [SkyOffset] is the rotation that actually moves the target to the origin,
// so offsets are measured on the sphere the whole way out and there is no
// small-angle assumption to outgrow.
//
// # What it is for
//
// Dither and mosaic patterns, offset guide stars, slit and detector layouts,
// finder charts, and any "put the target at the centre and tell me where
// everything else falls" question. The frame is a value: build one per target
// and apply it to as many positions as needed.
//
// # Orientation
//
// With a zero rotation the frame's +lat axis points north and its +lon axis
// points east, matching the position-angle convention [PositionAngle] uses. A
// non-zero rotation turns the frame so that **+lat points at that position
// angle** — set it to an instrument's position angle and lat becomes the
// along-slit coordinate, lon the across-slit one.
type SkyOffset struct {
	origin   ICRS
	rotation angle.Angle
}

// NewSkyOffset returns the offset frame centred on origin, with the frame's
// +lat axis pointing at the given position angle (north through east).
//
// Pass a zero rotation for a north-up, east-left frame.
func NewSkyOffset(origin ICRS, rotation angle.Angle) SkyOffset {
	return SkyOffset{origin: origin, rotation: rotation}
}

// Origin returns the position the frame is centred on.
func (f SkyOffset) Origin() ICRS { return f.origin }

// Rotation returns the position angle the frame's +lat axis points at.
func (f SkyOffset) Rotation() angle.Angle { return f.rotation }

// FromICRS returns the offsets of c from the frame's origin.
//
// lon is in (-180°, 180°] and increases eastward; lat is in [-90°, 90°] and
// increases northward, both turned by the frame's rotation. The origin itself
// maps to (0, 0).
//
// These are true spherical coordinates in a rotated frame, not a projection
// onto a plane, so they stay exact out to the antipode. The relations back to
// [Separation] and [PositionAngle] are
//
//	Separation(origin, c)    = acos(cos lat · cos lon)
//	PositionAngle(origin, c) = atan2(sin lon · cos lat, sin lat) + rotation
//
// and both hold at any separation. Note the second one: near the origin it
// degenerates to atan2(lon, lat), which is the form worth being careful about,
// since it is right to a part in 10⁸ at an arcminute and visibly wrong at a
// degree.
func (f SkyOffset) FromICRS(c ICRS) (lon, lat angle.Angle) {
	x, y := f.toFrame(c.ToUnitVector()).ToSpherical()

	return angle.Rad(x).Wrap180(), angle.Rad(y)
}

// ToICRS returns the ICRS position at the given offsets from the frame's
// origin. It is the exact inverse of [SkyOffset.FromICRS].
func (f SkyOffset) ToICRS(lon, lat angle.Angle) ICRS {
	v := f.toSky(vector.FromSpherical(lon.Radians(), lat.Radians()))

	var c ICRS
	c.FromUnitVector(v)

	return c
}

// String renders the frame for logs and errors.
func (f SkyOffset) String() string {
	return fmt.Sprintf("SkyOffset origin %s rotation %s", f.origin, f.rotation)
}

// toFrame rotates an ICRS unit vector into the offset frame.
//
// Three rotations, each of the frame rather than of the vector, which is why
// the signs are the negatives of the ones that would move a point the same
// way: about z by the origin's right ascension, bringing it into the x–z
// plane; about y by its declination, bringing it onto the x axis; then about
// x by the frame's rotation.
func (f SkyOffset) toFrame(v vector.Vec3) vector.Vec3 {
	return v.
		RotateZ(-f.origin.RA().Radians()).
		RotateY(f.origin.Dec().Radians()).
		RotateX(f.rotation.Radians())
}

// toSky is the inverse of [SkyOffset.toFrame]: the same three rotations
// negated and applied in the opposite order.
func (f SkyOffset) toSky(v vector.Vec3) vector.Vec3 {
	return v.
		RotateX(-f.rotation.Radians()).
		RotateY(-f.origin.Dec().Radians()).
		RotateZ(f.origin.RA().Radians())
}
