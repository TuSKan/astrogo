package coord_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/unit"
	"github.com/TuSKan/astrogo/vector"
)

// TestTheDerivationReproducesAstropysRotationalComponent is the check that
// makes deriving the solar velocity defensible rather than merely different.
//
// Astropy's Galactocentric frame defaults to galcen_v_sun = (12.9, 245.6, 7.78)
// km/s, citing Drimmel & Poggio (2018), and astrogo derives its rotational
// component instead — from Reid & Brunthaler's Sgr A* proper motion times the
// distance to the center.
//
// Those are not two routes to a shared constant. They are the same arithmetic:
// evaluated at astropy's own R₀ of 8122 pc, the derivation gives 245.6049,
// which is 245.6 to every digit astropy publishes. So astropy's V *is* R₀ × μ,
// and deriving it here is reconstructing how that number was obtained rather
// than departing from it.
//
// What the derivation buys is that V now tracks R₀. A frame built with one
// paper's distance and another's velocity is quietly inconsistent; this one
// cannot be.
func TestTheDerivationReproducesAstropysRotationalComponent(t *testing.T) {
	t.Parallel()

	const (
		astropyDistancePc = 8122.0
		astropyVKmPerS    = 245.6
	)

	got := coord.SolarVelocityFromSgrA(unit.Pc(astropyDistancePc))

	// Astropy publishes four significant figures, so agreement is asserted at
	// the precision they state and no further.
	if math.Abs(got.Y-astropyVKmPerS) > 0.05 {
		t.Errorf("at astropy's R0 the derived rotational component is %.4f km/s, "+
			"want their %.1f — the derivation no longer reconstructs their number",
			got.Y, astropyVKmPerS)
	}

	// The other two are Schönrich's rather than Drimmel & Poggio's, and differ
	// by more than rounding. That is a disagreement between papers, not an
	// error, and it is asserted so the gap cannot drift unnoticed.
	testutil.AssertNear(t, "radial component", got.X, 11.1, 1e-9)
	testutil.AssertNear(t, "vertical component", got.Z, 7.25, 1e-9)
}

// TestTheRotationalComponentScalesWithTheDistance pins the property that makes
// the derivation worth having: V is R₀ × μ, so it is proportional to R₀ while
// the other two components are not.
func TestTheRotationalComponentScalesWithTheDistance(t *testing.T) {
	t.Parallel()

	base := coord.SolarVelocityFromSgrA(unit.Pc(8000))
	twice := coord.SolarVelocityFromSgrA(unit.Pc(16000))

	testutil.AssertRelNear(t, "rotational component doubles with R0", twice.Y, 2*base.Y, 1e-12)

	// And the peculiar components do not move, because they are the Sun's own
	// wander with respect to the LSR and have nothing to do with how far away
	// the center is.
	testutil.AssertExact(t, "radial component is unchanged", twice.X, base.X)
	testutil.AssertExact(t, "vertical component is unchanged", twice.Z, base.Z)

	// A zero distance leaves only the peculiar motion, which is the right
	// answer rather than a degenerate one.
	if v := coord.SolarVelocityFromSgrA(0); v.Y != 0 {
		t.Errorf("at zero distance the rotational component is %g, want 0", v.Y)
	}
}

// TestAStarAtRestWithRespectToTheSunMovesWithIt is the clearest statement of
// what the velocity transform does.
//
// A star with no motion of its own relative to the Sun is nonetheless orbiting
// the Galaxy, at exactly the Sun's velocity. If the frame did not add the
// Sun's motion, such a star would come out stationary at the center of the
// Galaxy, which is the error this is guarding.
func TestAStarAtRestWithRespectToTheSunMovesWithIt(t *testing.T) {
	t.Parallel()

	f := coord.DefaultGalactocentricFrame()

	atRest := coord.NewICRSWithKinematics(
		angle.Deg(123.4), angle.Deg(-35.6), 0, 0, angle.Arcsec(0.020), 0)

	g := f.FromICRS(atRest, coord.ParallaxDistance(angle.Arcsec(0.020)))

	v, ok := g.Velocity()
	if !ok {
		t.Fatal("a star with a parallax and recorded kinematics has a velocity")
	}

	sunV, ok := f.SunPosition().Velocity()
	if !ok {
		t.Fatal("the Sun has a velocity in this frame; it is the frame parameter")
	}

	if diff := v.Sub(sunV).Norm(); diff > 1e-9 {
		t.Errorf("a star at rest with respect to the Sun differs from it by %g km/s, want 0", diff)
	}

	// And that shared velocity is the frame's parameter, tilted onto the
	// frame's axes — around 247.7 km/s, which is the Sun's orbital speed.
	testutil.AssertRelNear(t, "the Sun's speed in this frame", sunV.Norm(), 247.65, 1e-3)
}

// TestTheVelocityRoundTripsThroughTheFrame is the inverse property over all six
// elements, and the reason ToICRS reconstructs kinematics rather than dropping
// them.
//
// A conversion that loses the velocity on the way back is not an inverse, and
// the loss is silent — the position still round-trips perfectly, so every
// position-only test keeps passing.
func TestTheVelocityRoundTripsThroughTheFrame(t *testing.T) {
	t.Parallel()

	f := coord.DefaultGalactocentricFrame()

	for _, tc := range []struct {
		name        string
		ra, dec     float64 // degrees
		pmRA, pmDec float64 // arcsec/yr, on-sky
		px          float64 // arcsec
		rv          float64 // km/s
	}{
		{"Barnard's Star", 269.452, 4.693, -0.79847, 10.33777, 0.54698, -110.6},
		{"a typical field star", 123.4, -35.6, 0.020, -0.035, 0.012, -22.4},
		{"at rest at a known distance", 45, 78, 0, 0, 0.020, 0},
		{"southern, receding", 201.3, -43.1, -0.310, -0.480, 0.089, 45.0},
		{"across the RA wrap", 359.99, 10, 0.05, 0.05, 0.03, 12.5},
		{"near the south pole", 310.7, -81.2, 0.15, -0.22, 0.005, -180},
	} {
		star := coord.NewICRSWithKinematics(
			angle.Deg(tc.ra), angle.Deg(tc.dec),
			angle.Arcsec(tc.pmRA), angle.Arcsec(tc.pmDec),
			angle.Arcsec(tc.px), unit.KmPerSec(tc.rv),
		)

		distance := coord.ParallaxDistance(angle.Arcsec(tc.px))

		g := f.FromICRS(star, distance)
		if _, ok := g.Velocity(); !ok {
			t.Errorf("%s: no velocity survived FromICRS", tc.name)
			continue
		}

		back, backDistance := f.ToICRS(g)

		if sep := coord.Separation(star, back).Arcseconds(); sep > 1e-6 {
			t.Errorf("%s: direction moved %.3g arcsec", tc.name, sep)
		}

		testutil.AssertRelNear(t, tc.name+" distance", backDistance.Pc(), distance.Pc(), 1e-9)

		// The kinematics are the point: all four must come back, since the
		// velocity carries them jointly and a defect in the reconstruction
		// shows up in whichever one it touches.
		testutil.AssertNear(t, tc.name+" pmRA",
			back.PmRA().Arcseconds(), tc.pmRA, 1e-9)
		testutil.AssertNear(t, tc.name+" pmDec",
			back.PmDec().Arcseconds(), tc.pmDec, 1e-9)
		testutil.AssertRelNear(t, tc.name+" parallax",
			back.Parallax().Arcseconds(), tc.px, 1e-9)

		if math.Abs(back.RV().KmPerSec()-tc.rv) > 1e-6 {
			t.Errorf("%s: radial velocity came back %.9f km/s, want %.4f",
				tc.name, back.RV().KmPerSec(), tc.rv)
		}
	}
}

// TestNoParallaxMeansAPositionWithoutAVelocity checks that the two halves fail
// independently: a caller who supplies a distance but no parallax still gets a
// position, and gets told there is no velocity rather than being handed one.
func TestNoParallaxMeansAPositionWithoutAVelocity(t *testing.T) {
	t.Parallel()

	f := coord.DefaultGalactocentricFrame()

	for _, tc := range []struct {
		name string
		star coord.ICRS
	}{
		{
			name: "no kinematics recorded",
			star: coord.NewICRS(angle.Deg(123.4), angle.Deg(-35.6)),
		},
		{
			name: "proper motion but no parallax to scale it",
			star: coord.NewICRSWithKinematics(
				angle.Deg(123.4), angle.Deg(-35.6),
				angle.Arcsec(0.150), angle.Arcsec(0.220), 0, unit.KmPerSec(-22.4)),
		},
	} {
		g := f.FromICRS(tc.star, unit.Pc(1000))

		// The position is unaffected, because it needs the distance the caller
		// passed and not a parallax. Compared against the same direction with
		// nothing recorded at all, which is the position-only path.
		wantPos := f.FromICRS(coord.NewICRS(tc.star.RA(), tc.star.Dec()), unit.Pc(1000))

		if g.Vector() != wantPos.Vector() {
			t.Errorf("%s: position is %s, want the position-only answer %s", tc.name, g, wantPos)
		}

		if v, ok := g.Velocity(); ok {
			t.Errorf("%s: reported a velocity of %v km/s, want none", tc.name, v)
		}

		// And ToICRS gives back a position-only ICRS rather than inventing
		// kinematics for it.
		back, _ := f.ToICRS(g)
		if back.PmRA() != 0 || back.PmDec() != 0 || back.Parallax() != 0 || back.RV() != 0 {
			t.Errorf("%s: ToICRS invented kinematics: %v", tc.name, back)
		}
	}
}

// TestNewGalactocentricWithVelocityIsIndistinguishableFromAComputedOne covers
// the constructor, which is how a caller enters a velocity astrogo did not
// compute — a simulation snapshot, or a published Galactocentric state.
func TestNewGalactocentricWithVelocityIsIndistinguishableFromAComputedOne(t *testing.T) {
	t.Parallel()

	f := coord.DefaultGalactocentricFrame()

	star := coord.NewICRSWithKinematics(
		angle.Deg(269.452), angle.Deg(4.693),
		angle.Arcsec(-0.79847), angle.Arcsec(10.33777),
		angle.Arcsec(0.54698), unit.KmPerSec(-110.6))

	computed := f.FromICRS(star, coord.ParallaxDistance(angle.Arcsec(0.54698)))

	vel, ok := computed.Velocity()
	if !ok {
		t.Fatal("the computed position should carry a velocity")
	}

	rebuilt := coord.NewGalactocentricWithVelocity(
		computed.X(), computed.Y(), computed.Z(), vel)

	if rebuilt != computed {
		t.Errorf("rebuilt %s differs from computed %s", rebuilt, computed)
	}

	// A position built without a velocity says so, rather than carrying a zero
	// that reads as "not moving".
	if _, ok := coord.NewGalactocentric(unit.Pc(1), unit.Pc(2), unit.Pc(3)).Velocity(); ok {
		t.Error("NewGalactocentric produced a position claiming to have a velocity")
	}
}

// TestBarnardsStarGalactocentricVelocityIsPlausible is a coarse sanity check on
// the composed result, in the one place a wrong sign would otherwise hide.
//
// Barnard's Star moves 142.5 km/s with respect to the Sun, and the Sun moves
// about 248 km/s around the Galaxy. Its Galactocentric speed must therefore lie
// between the difference and the sum of those, and nowhere near either zero or
// 390 km/s — which is what adding the solar velocity with the wrong sign, or
// omitting it, would produce.
func TestBarnardsStarGalactocentricVelocityIsPlausible(t *testing.T) {
	t.Parallel()

	f := coord.DefaultGalactocentricFrame()

	star := coord.NewICRSWithKinematics(
		angle.Deg(269.452), angle.Deg(4.693),
		angle.Arcsec(-0.79847), angle.Arcsec(10.33777),
		angle.Arcsec(0.54698), unit.KmPerSec(-110.6))

	g := f.FromICRS(star, coord.ParallaxDistance(angle.Arcsec(0.54698)))

	v, ok := g.Velocity()
	if !ok {
		t.Fatal("Barnard's Star has a parallax of 547 mas and full kinematics")
	}

	sunSpeed, _ := f.SunPosition().Velocity()

	const relativeSpeed = 142.5

	low := sunSpeed.Norm() - relativeSpeed
	high := sunSpeed.Norm() + relativeSpeed

	if v.Norm() < low || v.Norm() > high {
		t.Errorf("Galactocentric speed %.1f km/s is outside [%.1f, %.1f], which the triangle "+
			"inequality requires for a star moving %.1f km/s relative to a Sun moving %.1f",
			v.Norm(), low, high, relativeSpeed, sunSpeed.Norm())
	}

	// It should also be rotating with the Galaxy rather than against it: a
	// retrograde result would mean the rotational component was subtracted.
	if v.Y <= 0 {
		t.Errorf("the rotational component is %.1f km/s, so the star is retrograde — "+
			"the Sun's motion was probably applied with the wrong sign", v.Y)
	}
}

// TestTheGalacticBasisIsOrthonormal checks the rotation the velocity transform
// is built on, which the position path never exercises because it converts
// directions through angles instead.
//
// An inexact basis would quietly stop preserving the length of every velocity
// it rotates, which is a defect no round trip would catch — the same error
// appears going both ways and cancels.
func TestTheGalacticBasisIsOrthonormal(t *testing.T) {
	t.Parallel()

	f := coord.DefaultGalactocentricFrame()

	// A rotation preserves length. Build an ICRS velocity by hand, put it
	// through the frame, and remove the Sun's contribution.
	sunV, _ := f.SunPosition().Velocity()

	for _, tc := range []struct {
		name    string
		ra, dec float64
		pmRA    float64
		pmDec   float64
		px      float64
		rv      float64
	}{
		{"mid sky", 123.4, -35.6, 0.150, 0.220, 0.020, -22.4},
		{"pole", 0, 89.9, 0.100, 0.100, 0.050, 10},
		{"equator", 45, 0, -0.200, 0.300, 0.030, -50},
	} {
		star := coord.NewICRSWithKinematics(
			angle.Deg(tc.ra), angle.Deg(tc.dec),
			angle.Arcsec(tc.pmRA), angle.Arcsec(tc.pmDec),
			angle.Arcsec(tc.px), unit.KmPerSec(tc.rv))

		bary, ok := coord.SpaceVelocity(star)
		if !ok {
			t.Fatalf("%s: no space velocity", tc.name)
		}

		g := f.FromICRS(star, coord.ParallaxDistance(angle.Arcsec(tc.px)))

		v, ok := g.Velocity()
		if !ok {
			t.Fatalf("%s: no frame velocity", tc.name)
		}

		// Whatever rotation was applied, it must not have changed the length
		// of the star's own contribution.
		rotated := v.Sub(sunV)

		testutil.AssertRelNear(t, tc.name+" speed preserved by the rotation",
			rotated.Norm(), bary.Norm(), 1e-12)
	}

	// The axes themselves: +Y must point toward Galactic longitude 90, which
	// is what makes the frame right-handed rather than mirrored.
	towardRotation := coord.GalacticToICRS(coord.NewGalactic(angle.Deg(90), 0))

	g := f.FromICRS(towardRotation, 1000)
	sun := f.SunPosition()

	if g.Y()-sun.Y() < 990 {
		t.Errorf("a target toward Galactic l = 90 is only %.1f pc along +Y from the Sun, "+
			"want about 1000 — the frame is mirrored", g.Y()-sun.Y())
	}
}

// TestToICRSDeclinesAVelocityItCannotSplit covers the two ways the inverse can
// fail to turn a Cartesian velocity back into catalogue kinematics.
//
// Both are positions a caller can construct and no star occupies, and in both
// the position is still returned — losing the velocity is the right degradation,
// returning nothing would not be, and inventing kinematics would be worst.
func TestToICRSDeclinesAVelocityItCannotSplit(t *testing.T) {
	t.Parallel()

	f := coord.DefaultGalactocentricFrame()
	sun := f.SunPosition()

	t.Run("a target at the Sun comes back at no distance", func(t *testing.T) {
		t.Parallel()

		atSun := coord.NewGalactocentricWithVelocity(
			sun.X(), sun.Y(), sun.Z(), vector.V3(10, 250, 5))

		_, distance := f.ToICRS(atSun)

		// Not exactly zero: the Sun's position is reconstructed through a
		// rotation and a translation, so it lands within a femtoparsec of the
		// origin rather than on it. What matters is that nothing blows up and
		// the distance is recognisably nothing.
		if distance.Pc() > 1e-9 {
			t.Errorf("the Sun's own position came back at distance %g pc, want essentially 0",
				distance.Pc())
		}
	})

	t.Run("a superluminal velocity is refused rather than split", func(t *testing.T) {
		t.Parallel()

		// Well past c, which SOFA's Pvstar reports as status -1.
		fast := coord.NewGalactocentricWithVelocity(
			1000, 2000, 300, vector.V3(1e6, 0, 0))

		back, distance := f.ToICRS(fast)

		if distance <= 0 {
			t.Fatalf("expected a real distance, got %g", distance)
		}

		if back.PmRA() != 0 || back.PmDec() != 0 || back.Parallax() != 0 || back.RV() != 0 {
			t.Errorf("kinematics were invented for a superluminal velocity: %v", back)
		}
	})
}
