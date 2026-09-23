package coord

import (
	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// The Sun's velocity in the Galactocentric frame, and why it is derived here
// rather than copied.
//
// # What is actually measured
//
// Three numbers are needed: the Sun's motion toward the Galactic center, along
// Galactic rotation, and toward the north Galactic pole. They do not come from
// one source, because they are not one kind of measurement.
//
// The **radial and vertical** components are the Sun's peculiar motion with
// respect to the Local Standard of Rest. The LSR is on a circular orbit by
// construction, so it has no radial and no vertical velocity of its own, and
// the Sun's components in those directions are therefore its peculiar ones
// entire: U☉ = 11.1 km/s and W☉ = 7.25 km/s from Schönrich, Binney & Dehnen
// (2010), which this package already cites for [LSRDynamical].
//
// The **rotational** component is different, and it is the one that cannot be
// taken from a peculiar-velocity paper, because it is dominated by the
// circular speed rather than by the Sun's wander. It is measured directly, and
// very precisely, by the apparent proper motion of Sgr A*: the black hole is
// effectively at rest at the Galactic center, so what is seen is the reflex of
// the Sun's own orbit. Reid & Brunthaler (2004), ApJ 616, 872, measure
//
//	6.379 ± 0.024 mas/yr, "almost entirely in the plane of the Galaxy"
//
// against two extragalactic radio sources over eight years. Multiplied by the
// distance, that is the Sun's transverse speed.
//
// # Why this is better than writing down a triple
//
// Because the rotational component is R₀ × μ, it is not independent of R₀ —
// and a frame built from one paper's R₀ and another's velocity is quietly
// inconsistent. Deriving it means the velocity tracks whatever distance the
// frame was given, so a caller reproducing somebody else's result gets a frame
// that hangs together rather than a mixture.
//
// It also makes the comparison with astropy exact rather than approximate.
// Astropy's Galactocentric defaults to galcen_v_sun = (12.9, 245.6, 7.78) km/s,
// citing Drimmel & Poggio (2018) — and 4.7404705 × 6.379 × 8.122 = 245.60,
// which is their rotational component to every digit they publish. Their V is
// this same derivation at their own R₀ of 8122 pc. [TestTheDerivationReproducesAstropysRotationalComponent]
// is that check.
//
// The other two components differ — 12.9 against 11.1 and 7.78 against 7.25 —
// because Drimmel & Poggio combine a different peculiar-velocity determination
// than Schönrich's. Those are 1.8 and 0.5 km/s, they are honest disagreements
// between papers rather than arithmetic, and [NewGalactocentricFrame] is how a
// caller who wants somebody else's numbers supplies them.

// Solar peculiar motion with respect to the Local Standard of Rest, from
// Schönrich, Binney & Dehnen (2010), MNRAS 403, 1829, table 3 — the same
// determination [LSRDynamical] uses, entered once and read from both places.
//
// The middle component of that paper's triple, V☉ = 12.24 km/s, is deliberately
// absent: it is the Sun's peculiar motion in the direction of rotation, and it
// is already inside the transverse speed Sgr A*'s proper motion measures. Using
// both would count it twice.
const (
	sunPeculiarUKmPerS = 11.1
	sunPeculiarWKmPerS = 7.25
)

// sgrAProperMotionMasPerYear is the apparent proper motion of Sgr A* along the
// Galactic plane: 6.379 ± 0.024 mas/yr (Reid & Brunthaler 2004, ApJ 616, 872).
//
// It is the reflex of the Sun's orbit, not a motion of the black hole. The same
// paper finds the residual perpendicular to the plane to be −0.4 ± 0.9 km/s,
// which is consistent with zero and is the evidence that Sgr A* is at rest at
// the center rather than merely near it.
const sgrAProperMotionMasPerYear = 6.379

// auPerYearInKmPerSec is one astronomical unit per Julian year expressed in
// km/s, the constant that turns an angular rate at a distance into a speed:
// a proper motion of μ arcsec/yr at d parsecs is 4.7404705·μ·d km/s.
//
// It is not an independent constant, and it is now *computed* rather than
// written down, because writing it down is how it came to be wrong.
//
// It shipped as the literal 4.740470446, which is the decimal repeated in a
// great deal of literature and is not the au divided by the Julian year. That
// quotient is 4.740470463533: 149597870700 m over 365.25 x 86400 s, both exact
// by definition since IAU 2012 Resolution B2. The doc comment beside the
// literal asserted the derivation the literal did not satisfy — which is
// exactly the failure "never fabricate a coefficient" is meant to prevent, and
// it went unnoticed because coord's own test encoded the same wrong decimal.
//
// The error was 3.7e-09 relative, so nothing observable moved: the Sun's
// rotational velocity in [SolarVelocityFromSgrA] shifts by 9.2e-07 km/s. It is
// corrected because a constant whose documentation describes a different number
// than the constant holds is a trap for whoever reads it next, not because the
// answer was wrong by anything a measurement could see.
//
// Deriving it from [constants] means it cannot drift from the au again, and
// removes the only place a reader had to trust a decimal.
var auPerYearInKmPerSec = kmPerAU / julianYearSeconds

// julianYearSeconds is the Julian year, which is 365.25 days of 86400 seconds
// exactly — a definition rather than a measurement, which is why it is a
// literal here and the au is not.
const julianYearSeconds = 365.25 * 86400

// SolarVelocityFromSgrA returns the Sun's velocity in the Galactocentric frame,
// in km/s, for a Galactic-center distance of sunDistance.
//
// The components are (toward the center, along rotation, toward the north
// Galactic pole), on the axes of the Galactic frame — [GalactocentricFrame]
// applies its own tilt to them, the same tilt it applies to positions.
//
// The rotational component is sunDistance × the Sgr A* proper motion, so it
// scales with the distance the caller supplies. The other two are the Sun's
// peculiar motion with respect to the LSR, which does not. See the commentary
// at the top of this file for why the three come from different places.
//
// Pass the same distance to this and to [NewGalactocentricFrame], or use
// [DefaultGalactocentricFrame], which does.
func SolarVelocityFromSgrA(sunDistance unit.Length) vector.Vec3 {
	// μ in arcsec/yr times distance in parsecs, times one au per year in km/s.
	rotation := auPerYearInKmPerSec * (sgrAProperMotionMasPerYear / 1000) * sunDistance.Pc()

	return vector.V3(sunPeculiarUKmPerS, rotation, sunPeculiarWKmPerS)
}

// galacticBasis holds the Galactic frame's axes as ICRS unit vectors, so a
// velocity can be rotated without going through spherical coordinates.
//
// A direction can be converted with [ICRSToGalactic], which is what the
// position path uses. A velocity cannot: it is a vector at a point rather than
// a point, and converting it through angles would mean differentiating the
// angles. Rotating the components directly is both exact and simpler.
//
// The axes come from [GalacticToICRS] rather than from a written-down matrix,
// so there is still one definition of which way the Galaxy points — the same
// reasoning [GalactocentricFrame] gives for not parameterising its orientation.
var galacticBasis = computeGalacticBasis()

// galacticAxes is the Galactic frame expressed on the ICRS axes.
type galacticAxes struct {
	x, y, z vector.Vec3
}

// computeGalacticBasis builds the Galactic axes from this package's own
// definition of the frame: +x toward Galactic (0, 0), +z toward the north
// Galactic pole, +y completing a right-handed set and so pointing toward
// l = 90°.
func computeGalacticBasis() galacticAxes {
	center := GalacticToICRS(NewGalactic(0, 0)).ToUnitVector()
	z := GalacticToICRS(NewGalactic(0, angle.Deg(90))).ToUnitVector()

	// Gram-Schmidt against the pole rather than trusting the two directions to
	// be exactly perpendicular. They are defined to be, and they arrive here
	// through two trigonometric conversions, so the residual is small but not
	// zero — and an inexact basis would quietly stop preserving the length of
	// every velocity it rotates.
	y := z.Cross(center).Unit()
	x := y.Cross(z).Unit()

	return galacticAxes{x: x, y: y, z: z}
}

// toGalactic expresses an ICRS vector on the Galactic axes.
func (a galacticAxes) toGalactic(v vector.Vec3) vector.Vec3 {
	return vector.V3(v.Dot(a.x), v.Dot(a.y), v.Dot(a.z))
}

// toICRS is the inverse: a Galactic vector back onto the ICRS axes.
func (a galacticAxes) toICRS(v vector.Vec3) vector.Vec3 {
	return a.x.MulScalar(v.X).Add(a.y.MulScalar(v.Y)).Add(a.z.MulScalar(v.Z))
}
