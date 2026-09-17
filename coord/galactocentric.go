package coord

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/vector"
)

// Galactocentric is a position in a right-handed Cartesian frame whose origin
// is the centre of the Galaxy, in parsecs.
//
//	+X points from the Sun toward the Galactic centre
//	+Y points in the direction of Galactic rotation (roughly Galactic l = 90°)
//	+Z points toward the north Galactic pole
//
// The Sun is therefore at negative X, a little above the plane. It is the frame
// Galactic structure is written in: a rotation curve is a function of the
// cylindrical radius [Galactocentric.Radius], a disc scale height is a spread
// in Z, and a stellar stream is a track through all three.
//
// # It is Cartesian on purpose
//
// Every other frame in this package is a direction with an optional distance,
// because that is what an observation is. This one is not, because the
// questions asked of it are not angular: "how far from the centre", "how far
// above the plane", "is this in the bar". Expressing those as a longitude and a
// latitude around the Galactic centre would be a faithful translation of an
// unhelpful shape.
//
// It also means the frame needs a real distance to be entered at all. A
// direction alone cannot be placed in it — see [GalactocentricFrame.FromICRS].
//
// # A velocity, when the target has one
//
// [GalactocentricFrame.FromICRS] attaches a velocity in km/s whenever the
// target's kinematics can supply one, and [Galactocentric.Velocity] reports
// whether it did. The two are carried together because they are one state: an
// orbit needs both, and a position that silently lost its velocity on the way
// back through [GalactocentricFrame.ToICRS] would be an inverse in name only.
//
// A velocity needs more than a position does — a parallax able to turn an
// angular rate into km/s, which a distance passed by the caller cannot
// substitute for. [SpaceVelocity] documents why, and why that is not the same
// requirement a frame conversion has.
type Galactocentric struct {
	// v holds (X, Y, Z) in parsecs.
	v vector.Vec3

	// vel holds the velocity in km/s on the same axes, and hasVelocity whether
	// there is one.
	//
	// A position always exists; a velocity does not. It needs the target's
	// kinematics and a parallax able to turn an angular rate into km/s, which
	// is the requirement [SpaceVelocity] documents and refuses without. The
	// flag keeps "no velocity available" distinct from "velocity is zero" —
	// the same distinction [ICRS.hasKinematics] draws, for the same reason.
	vel         vector.Vec3
	hasVelocity bool
}

// The two measured parameters of the default frame.
//
// Both are single published determinations rather than a weighted average of
// the literature, so that a reader can check astrogo against the paper rather
// than against a blend nobody published.
const (
	// sunGalacticDistancePc is R₀, the distance from the Sun to the Galactic
	// centre: 8178 ± 13 (stat) ± 22 (sys) pc, from GRAVITY Collaboration
	// (Abuter et al.) 2019, A&A 625, L10 — a geometric measurement from the
	// orbit of the star S2 around Sgr A*, with the orbit's angular size and
	// the star's radial velocity together fixing the scale.
	//
	// It is geometric, which is why it is the default here: it does not
	// descend from a standard candle, a rotation model or a calibration
	// ladder, so its error budget does not share terms with anything else
	// astrogo computes.
	//
	// astropy's Galactocentric defaults to 8122 pc, the same collaboration's
	// earlier result (GRAVITY Collaboration 2018, A&A 615, L15). The two
	// differ by 56 pc — 0.7%, and about two of the later measurement's
	// combined uncertainties. Anyone reproducing an astropy number should pass
	// 8122 to [NewGalactocentricFrame] rather than expect it here;
	// [TestAstropysDefaultFrameIsReproducible] is that comparison.
	sunGalacticDistancePc = 8178.0

	// sunMidplaneHeightPc is z☉, the Sun's height above the Galactic midplane:
	// 20.8 ± 0.3 pc, from Bennett & Bovy 2019, MNRAS 483, 1417, measured from
	// the vertical number-density profile of Gaia DR2 stars near the Sun.
	//
	// Small, and not negligible. It is a quarter of a percent of R₀, so a
	// frame built without it puts the Sun exactly in the plane and leaves the
	// frame rotated by 0.146° about its Y axis — which is 20.8 pc of Z error
	// at the far side of the disc. Not a large number, but a systematic one,
	// in the same direction for every star on that side of the Galaxy, and so
	// not something a larger sample averages away.
	sunMidplaneHeightPc = 20.8
)

// GalactocentricFrame is the frame [Galactocentric] positions are expressed in:
// where the Galactic centre is, and where the Sun sits relative to it.
//
// # What is a parameter here and what is not
//
// The two measured quantities are parameters, because measurements move: R₀ has
// walked from 8.5 kpc to 8.178 kpc within living memory and will move again,
// and a paper's numbers can only be reproduced in the frame that paper used.
//
// The *orientation* is not a parameter. The axes are those of the IAU Galactic
// frame that [ICRSToGalactic] already implements — the Hipparcos ICRS
// realisation of the 1958 system, via SOFA's Icrs2g. Letting a caller supply a
// different Galactic-centre direction would mean this file carried a second,
// independent definition of which way the Galaxy points, and the two could
// disagree. One definition, used twice, cannot.
//
// That choice is worth stating precisely, because it is where astrogo's frame
// and astropy's are defined differently even though they agree:
//
//   - astropy parameterises the Galactic-centre direction (galcen_coord) and a
//     roll angle, and its defaults are ICRS α = 266.4051°, δ = −28.936175° with
//     a roll of 58.5986320306°.
//   - astrogo takes the direction from [GalacticToICRS] of l = 0, b = 0, which
//     is α = 266.404995°, δ = −28.936174°, and the roll falls out of the same
//     rotation rather than being written down.
//
// The directions differ by a third of an arcsecond and the rolls by an eighth
// of one, which is astropy's own rounding of the same convention rather than a
// disagreement about it. At 8 kpc, a third of an arcsecond is 0.013 pc.
//
// Neither is the *radio* position of Sgr A*, and that is not an error in
// either. The IAU froze Galactic l = 0 in 1958 from the best available radio
// continuum, and the black hole was later found 4.3 arcmin away from it (Reid &
// Brunthaler 2004, ApJ 616, 872). The convention did not move, because a
// coordinate system that shifts whenever the measurement improves is not a
// coordinate system. If you need Sgr A* itself rather than Galactic l = 0,
// convert its ICRS position like any other target.
type GalactocentricFrame struct {
	sunDistance float64     // R₀, parsecs.
	sunHeight   float64     // z☉, parsecs.
	sunVelocity vector.Vec3 // The Sun's velocity in this frame, km/s.
}

// NewGalactocentricFrame returns the frame in which the Galactic centre lies
// sunDistance parsecs from the Sun and the Sun lies sunHeight parsecs above the
// Galactic midplane.
//
// Use it to reproduce a result computed in some other frame — a paper's R₀, or
// astropy's 8122 pc default. For ordinary work call
// [DefaultGalactocentricFrame], which carries the values this package cites.
//
// Nothing is validated. A sunDistance of zero leaves the frame untilted and
// centred on the Sun, and a sunHeight larger than sunDistance is clamped to a
// quarter turn; both are nonsense that a caller can construct, and neither
// produces a NaN that would propagate silently into a catalogue.
func NewGalactocentricFrame(sunDistance, sunHeight float64, sunVelocity vector.Vec3) GalactocentricFrame {
	return GalactocentricFrame{
		sunDistance: sunDistance,
		sunHeight:   sunHeight,
		sunVelocity: sunVelocity,
	}
}

// DefaultGalactocentricFrame returns the frame built from the values this
// package cites: R₀ = 8178 pc (GRAVITY Collaboration 2019) and z☉ = 20.8 pc
// (Bennett & Bovy 2019).
func DefaultGalactocentricFrame() GalactocentricFrame {
	return NewGalactocentricFrame(
		sunGalacticDistancePc,
		sunMidplaneHeightPc,
		SolarVelocityFromSgrA(sunGalacticDistancePc),
	)
}

// SunVelocity returns the Sun's velocity in this frame, in km/s, on the
// Galactic axes the caller supplied it on — see [SolarVelocityFromSgrA].
func (f GalactocentricFrame) SunVelocity() vector.Vec3 { return f.sunVelocity }

// SunDistance returns R₀, the Sun-to-Galactic-centre distance in parsecs.
func (f GalactocentricFrame) SunDistance() float64 { return f.sunDistance }

// SunHeight returns z☉, the Sun's height above the Galactic midplane in parsecs.
func (f GalactocentricFrame) SunHeight() float64 { return f.sunHeight }

// SunPosition returns where the Sun sits in this frame.
//
// It is (−√(R₀² − z☉²), 0, z☉), not (−R₀, 0, z☉). R₀ is the distance between
// the Sun and the centre, so once the Sun is lifted z☉ above the midplane its
// in-plane separation from the centre has to shrink for the total to stay R₀.
// The difference is 0.03 pc at the default parameters — far too small to matter
// and far too easy to get backwards, which is why it is computed rather than
// assumed.
// It also carries the Sun's velocity, which is the frame parameter itself
// tilted onto the frame's axes. That cannot come from [FromICRS], because the
// Sun has no parallax and no proper motion of its own to measure — its motion
// in this frame is the thing the frame is told, not a thing derived.
func (f GalactocentricFrame) SunPosition() Galactocentric {
	out := f.FromICRS(ICRS{}, 0)

	out.vel = f.sunVelocity.RotateY(f.tilt())
	out.hasVelocity = true

	return out
}

// FromICRS places a target at the given distance, in parsecs, into the frame.
//
// The distance is a separate argument rather than being read from c.Dist()
// because [ICRS.Dist] carries no unit of its own — it holds astronomical units
// on an ephemeris path and kilometres on a satellite one — and a frame measured
// in parsecs cannot be handed a number whose unit depends on where it came
// from. Naming it at the call site is the whole guard against that.
//
// For a target with a measured parallax, the distance is
// [ParallaxDistance](c.Parallax()).
//
// Only the direction of c is used. Any kinematics it carries are ignored, since
// this frame holds no velocity — see [Galactocentric].
func (f GalactocentricFrame) FromICRS(c ICRS, distance float64) Galactocentric {
	// Heliocentric Galactic Cartesian: the direction in the Galactic frame,
	// scaled out to the distance given.
	v := ICRSToGalactic(c).ToUnitVector().MulScalar(distance)

	// Move the origin to the Galactic centre, which lies R₀ away along +X, and
	// then tilt so the midplane passes through the centre with the Sun above
	// it rather than in it.
	out := Galactocentric{v: vector.V3(v.X-f.sunDistance, v.Y, v.Z).RotateY(f.tilt())}

	// The velocity, when the target carries enough to have one.
	//
	// Unlike the position it is not translated — an origin has no velocity —
	// but the frames are in relative motion, so the Sun's own velocity is
	// added. That is the whole of the transformation: rotate onto the Galactic
	// axes, tilt with the frame, and add the velocity the Sun itself has.
	if bary, ok := SpaceVelocity(c); ok {
		out.vel = galacticBasis.toGalactic(bary).RotateY(f.tilt()).
			Add(f.sunVelocity.RotateY(f.tilt()))
		out.hasVelocity = true
	}

	return out
}

// ToICRS returns the ICRS direction of g as seen from the Sun, and its distance
// in parsecs. It is the exact inverse of [GalactocentricFrame.FromICRS].
//
// The Sun's own position returns a zero distance and an arbitrary direction:
// there is no direction from a point to itself, and [vector.Vec3.ToSpherical]
// answers (0, 0) rather than a NaN.
func (f GalactocentricFrame) ToICRS(g Galactocentric) (c ICRS, distance float64) {
	// Undo the tilt, then put the origin back on the Sun.
	v := g.v.RotateY(-f.tilt())
	v = vector.V3(v.X+f.sunDistance, v.Y, v.Z)

	lon, lat := v.ToSpherical()

	out := GalacticToICRS(NewGalactic(angle.Rad(lon).Wrap360(), angle.Rad(lat)))
	distance = v.Norm()

	// A velocity, if there is one, comes back as catalogue kinematics: the
	// reverse of what [GalactocentricFrame.FromICRS] did, then SOFA's Pvstar
	// to split a Cartesian velocity back into a proper motion, a parallax and
	// a radial velocity.
	if vel, ok := g.Velocity(); ok {
		if kin, ok := icrsFromBarycentricVelocity(out, distance,
			galacticBasis.toICRS(vel.Sub(f.sunVelocity.RotateY(f.tilt())).RotateY(-f.tilt())),
		); ok {
			return kin, distance
		}
	}

	return out, distance
}

// icrsFromBarycentricVelocity rebuilds catalogue kinematics from a direction, a
// distance in parsecs and a barycentric velocity in km/s on the ICRS axes.
//
// It is the inverse of [SpaceVelocity], and exists so that
// [GalactocentricFrame.ToICRS] is a real inverse of FromICRS rather than one
// that quietly drops the velocity — which is the asymmetry that turns into a
// bug the first time somebody round-trips a catalogue through the frame.
//
// The bool is false when SOFA declines the pv-vector: a speed at or past c, or
// a zero-length position — which is a target at the Sun, where there is no
// direction and so no proper motion to report. Both mean the caller built
// something no star is, and both are left to Pvstar's own status rather than
// pre-checked here, since a distance of zero produces exactly the null position
// vector it already refuses.
func icrsFromBarycentricVelocity(dir ICRS, distancePc float64, velocity vector.Vec3) (ICRS, bool) {
	position := dir.ToUnitVector().MulScalar(distancePc * auPerParsec)
	perDay := velocity.MulScalar(secondsPerDay / kmPerAU)

	ra, dec, pmr, pmd, px, rv, status := gofaext.Pvstar([2][3]float64{
		{position.X, position.Y, position.Z},
		{perDay.X, perDay.Y, perDay.Z},
	})
	if status != 0 {
		return ICRS{}, false
	}

	return NewICRSWithKinematics(
		angle.Rad(ra).Wrap360(), angle.Rad(dec),
		pmRACosDec(pmr, angle.Rad(dec)), angle.Rad(pmd),
		angle.Arcsec(px), rv,
	), true
}

// auPerParsec is the number of astronomical units in a parsec.
//
// It is the definition of the parsec rather than a measurement — the distance
// at which one au subtends one arcsecond — so it is exactly 648000/π and is
// written that way instead of as a decimal somebody has to check.
const auPerParsec = 648000 / math.Pi

// tilt returns the angle the frame is rotated about the Y axis to lift the Sun
// z☉ above the midplane, in radians.
//
// The Galactic centre is at Galactic (l, b) = (0, 0) by definition, so the
// Sun–centre line lies in the Galactic b = 0 plane and the midplane cannot. The
// tilt is the angle between them: asin(z☉/R₀), about 0.146° by default.
//
// Zero distance gives zero rather than a NaN from 0/0, and a ratio past ±1 is
// clamped rather than handed to Asin outside its domain. Both are unphysical
// frames a caller can nonetheless construct, and a NaN here would spread into
// every coordinate computed from it without ever raising an error.
func (f GalactocentricFrame) tilt() float64 {
	if f.sunDistance == 0 {
		return 0
	}

	return math.Asin(min(1, max(-1, f.sunHeight/f.sunDistance)))
}

// NewGalactocentric builds a position from its Cartesian components, in parsecs.
func NewGalactocentric(x, y, z float64) Galactocentric {
	return Galactocentric{v: vector.V3(x, y, z)}
}

// NewGalactocentricWithVelocity builds a position carrying a velocity, the
// position in parsecs and the velocity in km/s on the same axes.
func NewGalactocentricWithVelocity(x, y, z float64, velocity vector.Vec3) Galactocentric {
	return Galactocentric{v: vector.V3(x, y, z), vel: velocity, hasVelocity: true}
}

// Velocity returns the velocity in km/s on this frame's axes, and whether there
// is one.
//
// It is false when the target it came from recorded no kinematics, or recorded
// them without a parallax able to scale an angular rate into km/s — see
// [SpaceVelocity], which makes the same distinction and explains why a frame
// conversion can manage without a distance while this cannot.
func (c Galactocentric) Velocity() (vector.Vec3, bool) { return c.vel, c.hasVelocity }

// X returns the component toward the Galactic centre, in parsecs. The Sun is at
// negative X.
func (c Galactocentric) X() float64 { return c.v.X }

// Y returns the component along Galactic rotation, in parsecs.
func (c Galactocentric) Y() float64 { return c.v.Y }

// Z returns the component toward the north Galactic pole, in parsecs. This is
// height above the midplane.
func (c Galactocentric) Z() float64 { return c.v.Z }

// Vector returns the position as a vector, in parsecs.
func (c Galactocentric) Vector() vector.Vec3 { return c.v }

// Distance returns the straight-line distance from the Galactic centre, in
// parsecs — the length of the full three-dimensional vector.
//
// For anything in or near the disc this is the wrong quantity to reach for and
// [Galactocentric.Radius] is the right one; they differ by less than a part in
// 10⁴ at the Sun, and by a great deal for a halo star. The distinction matters
// because the Galaxy is flat: its dynamics are organised by cylindrical radius
// and height separately, not by spherical radius.
func (c Galactocentric) Distance() float64 { return c.v.Norm() }

// Radius returns the cylindrical galactocentric radius √(X² + Y²), in parsecs:
// the distance from the Galaxy's rotation axis, measured in the midplane.
//
// This is the R of a rotation curve, of a disc surface-density profile, and of
// a metallicity gradient. The Sun's is √(R₀² − z☉²) rather than R₀ — see
// [GalactocentricFrame.SunPosition].
func (c Galactocentric) Radius() float64 { return math.Hypot(c.v.X, c.v.Y) }

// String renders the position for logs and errors.
func (c Galactocentric) String() string {
	return fmt.Sprintf("Galactocentric X %.3f Y %.3f Z %.3f pc", c.v.X, c.v.Y, c.v.Z)
}
