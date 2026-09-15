package coord

import (
	"fmt"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/vector"
)

// Supergalactic is a direction in the supergalactic coordinate system, whose
// equator follows the flattened plane the nearby bright galaxies lie in.
//
// # What it is for
//
// The galaxies within about 100 Mpc are not spread evenly. They are
// concentrated into a sheet — the Local Supercluster, with the Virgo cluster
// near its centre — and de Vaucouleurs defined this system to put that sheet
// on the equator, the way Galactic coordinates put the Milky Way's disc on
// theirs. Plotted in supergalactic latitude, a structure that looks like
// scatter on the sky becomes a band.
//
// It is a fixed rotation of Galactic coordinates and carries no epoch: the
// plane is defined by where galaxies are, not by where the Earth is pointing.
//
// # The definition
//
// The north supergalactic pole sits at Galactic l = 47.37°, b = +6.32°, and
// supergalactic longitude is zero at Galactic l = 137.37°, b = 0 — the
// de Vaucouleurs values, as [Supergalactic] and astropy both take them from
// Lahav et al. (2000), MNRAS 312, 166.
//
// The pole's b = +6.32° is worth noticing: the supergalactic pole lies close
// to the *Galactic plane*, so the two planes are nearly perpendicular. That is
// a fact about the sky rather than a convention, and it is why the Local
// Supercluster is cut in half by the zone of avoidance — the part of the sheet
// behind the Milky Way's own dust is the part hardest to survey.
type Supergalactic struct {
	sgl angle.Angle
	sgb angle.Angle
}

// The pole and longitude origin, in degrees of Galactic longitude and
// latitude. Entered as the literature states them and used nowhere else, so
// the rotation below is readable against its own source.
const (
	nsgpGalacticL = 47.37
	nsgpGalacticB = 6.32

	// Supergalactic longitude zero, at Galactic l = 137.37°, b = 0. It is a
	// quarter turn from the pole's longitude, which is what the final rotation
	// about the new pole below amounts to.
	sglOriginGalacticL = nsgpGalacticL + 90
)

// NewSupergalactic builds a supergalactic direction.
func NewSupergalactic(sgl, sgb angle.Angle) Supergalactic {
	return Supergalactic{sgl: sgl, sgb: sgb}
}

// SGL returns the supergalactic longitude.
func (c Supergalactic) SGL() angle.Angle { return c.sgl }

// SGB returns the supergalactic latitude. Near zero is the supercluster plane.
func (c Supergalactic) SGB() angle.Angle { return c.sgb }

// String renders the direction for logs and errors.
func (c Supergalactic) String() string {
	return fmt.Sprintf("Supergalactic SGL %s SGB %s", c.sgl, c.sgb)
}

// ToUnitVector converts the direction to a unit vector.
func (c Supergalactic) ToUnitVector() vector.Vec3 {
	return vector.FromSpherical(c.sgl.Radians(), c.sgb.Radians())
}

// FromUnitVector converts the unit vector to the direction.
func (c *Supergalactic) FromUnitVector(v vector.Vec3) {
	lon, lat := v.ToSpherical()
	c.sgl = angle.Rad(lon)
	c.sgb = angle.Rad(lat)
}

// ICRSToSupergalactic converts an ICRS direction to supergalactic coordinates.
//
// It routes through Galactic, because that is where the system is defined —
// there is no published ICRS pole for it, only a Galactic one, and going
// directly would mean writing down a rotation nobody else has checked.
func ICRSToSupergalactic(c ICRS) Supergalactic {
	var out Supergalactic

	out.FromUnitVector(galacticToSupergalacticVec(ICRSToGalactic(c).ToUnitVector()))

	out.sgl = out.sgl.Wrap360()

	return out
}

// SupergalacticToICRS converts a supergalactic direction to ICRS.
func SupergalacticToICRS(c Supergalactic) ICRS {
	var gal Galactic

	gal.FromUnitVector(supergalacticToGalacticVec(c.ToUnitVector()))

	return GalacticToICRS(NewGalactic(gal.L().Wrap360(), gal.B()))
}

// galacticToSupergalacticVec rotates a Galactic unit vector into the
// supergalactic frame.
//
// Three rotations of the frame rather than of the vector, which is why the
// signs are the negatives of the ones that would move a point the same way:
// about z by the pole's Galactic longitude, bringing it into the x–z plane;
// about y by the complement of its latitude, bringing it onto the z axis; then
// about z again, by the quarter turn that puts supergalactic longitude zero
// where the literature puts it.
func galacticToSupergalacticVec(v vector.Vec3) vector.Vec3 {
	return v.
		RotateZ(-angle.Deg(nsgpGalacticL).Radians()).
		RotateY(angle.Deg(nsgpGalacticB - 90).Radians()).
		RotateZ(-angle.Deg(sglOriginGalacticL - nsgpGalacticL).Radians())
}

// supergalacticToGalacticVec is the inverse: the same three rotations negated
// and applied in the opposite order.
func supergalacticToGalacticVec(v vector.Vec3) vector.Vec3 {
	return v.
		RotateZ(angle.Deg(sglOriginGalacticL - nsgpGalacticL).Radians()).
		RotateY(angle.Deg(90 - nsgpGalacticB).Radians()).
		RotateZ(angle.Deg(nsgpGalacticL).Radians())
}
