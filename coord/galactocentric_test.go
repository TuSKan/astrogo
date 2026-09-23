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

// galacticCenter is the ICRS direction the frame is built around: Galactic
// l = 0, b = 0, which is where the IAU convention puts the Galactic center.
func galacticCenter() coord.ICRS {
	return coord.GalacticToICRS(coord.NewGalactic(0, 0))
}

// TestSunSitsWhereTheFrameParametersPutIt is the frame's own definition read
// back: the Sun is R₀ from the center and z☉ above the midplane, and those two
// facts together fix its X.
//
// The X is the part worth asserting. −R₀ is the answer everyone writes down
// first and it is wrong by z☉²/2R₀, because R₀ is a distance and not an
// in-plane coordinate. The error is 0.026 pc here, which is small enough to
// survive review and large enough to be a real discrepancy against any tool
// that gets it right.
func TestSunSitsWhereTheFrameParametersPutIt(t *testing.T) {
	t.Parallel()

	f := coord.DefaultGalactocentricFrame()
	sun := f.SunPosition()

	r0, z0 := f.SunDistance().Pc(), f.SunHeight().Pc()
	wantX := -math.Sqrt(r0*r0 - z0*z0)

	testutil.AssertNear(t, "Sun X", sun.X().Pc(), wantX, 1e-9)
	testutil.AssertNear(t, "Sun Y", sun.Y().Pc(), 0, 1e-9)
	testutil.AssertNear(t, "Sun Z", sun.Z().Pc(), z0, 1e-9)

	// The two derived radii differ, and by the amount the frame says.
	testutil.AssertNear(t, "Sun distance from the center", sun.Distance().Pc(), r0, 1e-9)
	testutil.AssertNear(t, "Sun cylindrical radius", sun.Radius().Pc(), -wantX, 1e-9)

	if gap := r0 - sun.Radius().Pc(); gap < 0.02 || gap > 0.04 {
		t.Errorf("R₀ − cylindrical radius = %.6f pc, expected the 0.026 pc the geometry implies", gap)
	}
}

// TestGalacticCenterIsTheOrigin checks the one position the frame exists to
// name. A target in the direction of Galactic (0, 0) at exactly R₀ is the
// center, so it must land on (0, 0, 0) — after both the translation and the
// tilt, which is what makes this more than a subtraction.
func TestGalacticCenterIsTheOrigin(t *testing.T) {
	t.Parallel()

	f := coord.DefaultGalactocentricFrame()
	gc := f.FromICRS(galacticCenter(), f.SunDistance())

	// A part in 10¹² of R₀, which is floating-point noise on an 8178 pc
	// cancellation rather than a tolerance on the physics.
	const tol = 1e-8

	testutil.AssertNear(t, "center X", gc.X().Pc(), 0, tol)
	testutil.AssertNear(t, "center Y", gc.Y().Pc(), 0, tol)
	testutil.AssertNear(t, "center Z", gc.Z().Pc(), 0, tol)
	testutil.AssertNear(t, "center distance", gc.Distance().Pc(), 0, tol)
}

// TestAxesPointWhereTheDocumentationSays pins the handedness and the axis
// assignments, which are the part of a Cartesian frame that a sign error hides
// in: every other test in this file is symmetric under flipping Y, and a
// left-handed frame would pass all of them.
//
// The displacement from the Sun is what is measured, so the translation is out
// of the way and only the axes are under test.
//
// # Why the off-axis tolerance is 2.6 pc and not zero
//
// The frame is tilted by asin(z☉/R₀) = 0.1457°, so a displacement along a
// Galactic axis is not along a Galactocentric one: 1000 pc toward the center
// also moves −2.543 pc in Z, and 1000 pc toward the pole moves +2.543 pc in X.
// That is d·sin(tilt), it is the frame working, and a test that demanded zero
// there would be asserting the tilt away.
//
// Y is exempt because the tilt is about Y, so the two Galactic axes
// perpendicular to it stay exactly perpendicular to it.
func TestAxesPointWhereTheDocumentationSays(t *testing.T) {
	t.Parallel()

	f := coord.DefaultGalactocentricFrame()
	sun := f.SunPosition()

	const d = 1000 // parsecs

	// The tilt's own leakage between X and Z, plus a little room.
	tilt := math.Asin(f.SunHeight().Pc() / f.SunDistance().Pc())
	offAxis := d*math.Sin(tilt) + 0.01

	for _, tc := range []struct {
		name       string
		l, b       float64 // Galactic degrees.
		wx, wy, wz float64 // Expected displacement from the Sun, parsecs.
		why        string
		exactInY   bool
	}{
		{
			name: "toward the center raises X",
			l:    0, b: 0,
			wx: d, wy: 0, wz: 0,
			why:      "+X points from the Sun toward the Galactic center",
			exactInY: true,
		},
		{
			name: "Galactic rotation raises Y",
			l:    90, b: 0,
			wx: 0, wy: d, wz: 0,
			why: "+Y points along Galactic rotation, toward l = 90°",
		},
		{
			name: "the north Galactic pole raises Z",
			l:    0, b: 90,
			wx: 0, wy: 0, wz: d,
			why:      "+Z points toward the north Galactic pole",
			exactInY: true,
		},
		{
			name: "the anticenter lowers X",
			l:    180, b: 0,
			wx: -d, wy: 0, wz: 0,
			why:      "the anticenter is the other way along X",
			exactInY: true,
		},
	} {
		icrs := coord.GalacticToICRS(coord.NewGalactic(angle.Deg(tc.l), angle.Deg(tc.b)))
		got := f.FromICRS(icrs, unit.Pc(d))

		dx := (got.X() - sun.X()).Pc()
		dy := (got.Y() - sun.Y()).Pc()
		dz := (got.Z() - sun.Z()).Pc()

		// The named axis carries the whole displacement, to within the cosine
		// of the tilt — a part in 3×10⁶ of d, so 0.01 pc is generous.
		ok := math.Abs(dx-tc.wx) <= offAxis &&
			math.Abs(dy-tc.wy) <= offAxis &&
			math.Abs(dz-tc.wz) <= offAxis

		// Y is untouched by a rotation about Y.
		if tc.exactInY {
			ok = ok && math.Abs(dy) < 1e-9
		}

		if !ok {
			t.Errorf("%s: displacement from the Sun (%.3f, %.3f, %.3f) pc, want (%.0f, %.0f, %.0f) ± %.3f — %s",
				tc.name, dx, dy, dz, tc.wx, tc.wy, tc.wz, offAxis, tc.why)
		}
	}
}

// TestRoundTripThroughTheFrame is the inverse property over the whole sky and a
// wide range of distances: FromICRS then ToICRS must return the direction and
// the distance it was given.
//
// It covers the poles and the RA wrap, and distances from inside the solar
// neighbourhood to well past the far edge of the disc — including targets on
// the far side of the center, where X changes sign and the direction back to
// the Sun is nearly antiparallel to the one out.
//
// # The distance tolerance is absolute, and has to be
//
// A relative tolerance is the wrong contract here and fails for a correct
// implementation. Going in, the frame subtracts R₀ from a coordinate of order
// d; coming back it adds R₀ again. For a star 1 pc away that is 1 − 8178
// followed by +8178, so four significant digits of the 1 are lost and restored,
// leaving about 1e-12 pc of residue — 1e-12 pc absolutely, but 1e-12
// *relatively*, which no sensible relative tolerance would admit.
//
// The residue is set by the frame's own scale, not by the target's distance, so
// the contract is stated that way: a nanoparsec, at any distance. That is about
// 30 km, and seven orders of magnitude finer than the 0.013 pc by which the
// orientation convention itself is merely a rounding.
func TestRoundTripThroughTheFrame(t *testing.T) {
	t.Parallel()

	f := coord.DefaultGalactocentricFrame()

	distances := []float64{
		1,      // a nearby star
		100,    // the solar neighbourhood
		1000,   // a kiloparsec
		8178,   // the center's distance, so the far-side cases straddle it
		20000,  // past the far edge of the disc
		100000, // the halo
	}

	for _, ra := range []float64{0, 45, 123.456, 180, 266.4, 359.999} {
		for _, dec := range []float64{-90, -60, -28.9, 0, 27.1, 60, 90} {
			in := coord.NewICRS(angle.Deg(ra), angle.Deg(dec))

			for _, d := range distances {
				out, back := f.ToICRS(f.FromICRS(in, unit.Pc(d)))

				if gap := math.Abs(back.Pc() - d); gap > 1e-9 {
					t.Errorf("ra=%g dec=%g d=%g: round trip moved the distance by %.3g pc",
						ra, dec, d, gap)
				}

				// Separation rather than component differences: at a pole RA
				// is arbitrary and comparing it would fail on a frame that is
				// perfectly correct.
				//
				// The angular residue is the nanoparsec above divided by d, so
				// the nearest case here — 1 pc — is the worst one, at about
				// 2e-7 arcsec.
				sep := coord.Separation(in, out).Arcseconds()
				if sep > 1e-5 {
					t.Errorf("ra=%g dec=%g d=%g: round trip moved the direction by %.3g arcsec",
						ra, dec, d, sep)
				}
			}
		}
	}
}

// TestAstropysDefaultFrameIsReproducible checks astrogo's frame against the
// one other widely-used implementation of it, on the two things the comparison
// can actually settle: the orientation, and the Sun's place in it.
//
// The orientation is the interesting half. astrogo never writes the
// Galactic-center direction or the roll angle down — both fall out of
// [coord.ICRSToGalactic] — while astropy states them as frame parameters. That
// they agree is a check of astrogo's construction against an independent one,
// not a restatement of a shared constant.
//
// The residuals are astropy's rounding of the same convention: it publishes
// galcen_coord to six decimal places and roll0 to ten, and a third of an
// arcsecond is what the first of those costs.
func TestAstropysDefaultFrameIsReproducible(t *testing.T) {
	t.Parallel()

	// astropy.coordinates.Galactocentric defaults, as documented:
	// galcen_coord = ICRS(ra=266.4051°, dec=-28.936175°),
	// roll0 = 58.5986320306°, galcen_distance = 8.122 kpc, z_sun = 20.8 pc.
	const (
		astropyGalcenRA   = 266.4051
		astropyGalcenDec  = -28.936175
		astropyRoll0      = 58.5986320306
		astropyDistancePc = 8122.0
		astropyZSunPc     = 20.8
	)

	gc := galacticCenter()

	sep := coord.Separation(gc, coord.NewICRS(
		angle.Deg(astropyGalcenRA), angle.Deg(astropyGalcenDec),
	)).Arcseconds()
	if sep > 0.5 {
		t.Errorf("Galactic (0,0) is %.4f arcsec from astropy's galcen_coord, want under 0.5", sep)
	}

	// astropy's roll0 rotates its intermediate frame's Z onto the north
	// Galactic pole. Measured from the Galactic center, that is the pole's
	// position angle taken the other way round the circle.
	ngp := coord.GalacticToICRS(coord.NewGalactic(0, angle.Deg(90)))

	roll := 360 - coord.PositionAngle(gc, ngp).Wrap360().Degrees()
	if diff := math.Abs(roll-astropyRoll0) * 3600; diff > 0.5 {
		t.Errorf("derived roll %.10f° is %.4f arcsec from astropy's roll0 %.10f°, want under 0.5",
			roll, diff, astropyRoll0)
	}

	// With astropy's parameters, the Sun lands where astropy puts it.
	sun := coord.NewGalactocentricFrame(unit.Pc(astropyDistancePc), unit.Pc(astropyZSunPc),
		coord.SolarVelocityFromSgrA(unit.Pc(astropyDistancePc))).SunPosition()

	testutil.AssertNear(t, "astropy-frame Sun X", sun.X().Pc(),
		-math.Sqrt(astropyDistancePc*astropyDistancePc-astropyZSunPc*astropyZSunPc), 1e-9)
	testutil.AssertNear(t, "astropy-frame Sun Z", sun.Z().Pc(), astropyZSunPc, 1e-9)
}

// TestTheSunsHeightIsNotCosmetic measures the claim made in the doc comment on
// sunMidplaneHeightPc: dropping z☉ is not a rounding, it is a systematic Z
// error that grows across the Galaxy and reaches z☉ itself at the far side.
//
// Worth asserting rather than asserting z☉ is merely "used", because a frame
// that ignored it would still pass every round-trip test in this file.
func TestTheSunsHeightIsNotCosmetic(t *testing.T) {
	t.Parallel()

	withHeight := coord.DefaultGalactocentricFrame()
	flat := coord.NewGalactocentricFrame(withHeight.SunDistance(), 0, withHeight.SunVelocity())

	// A target in the Galactic plane on the far side of the center, twice as
	// far away as the center is.
	target := galacticCenter()
	d := 2 * withHeight.SunDistance()

	tilted, untilted := withHeight.FromICRS(target, d), flat.FromICRS(target, d)

	gap := math.Abs(tilted.Z().Pc() - untilted.Z().Pc())
	if !testutil.InRelTol(gap, withHeight.SunHeight().Pc(), 1e-6) {
		t.Errorf("Z differs by %.4f pc at the far side of the disc, expected z☉ = %.4f pc",
			gap, withHeight.SunHeight().Pc())
	}

	// And at the Sun the two frames differ by z☉ as well, in the other
	// direction — so the error is not a constant offset that cancels.
	s := math.Abs(withHeight.SunPosition().Z().Pc() - flat.SunPosition().Z().Pc())
	if !testutil.InRelTol(s, withHeight.SunHeight().Pc(), 1e-9) {
		t.Errorf("the Sun's Z differs by %.4f pc between the frames, expected z☉", s)
	}
}

// TestDegenerateFramesDoNotProduceNaN covers the two frames a caller can build
// that have no physical meaning. Neither is worth an error return, and both
// would otherwise reach math.Asin as 0/0 or as a ratio past 1 and return a NaN
// that spreads through every coordinate derived from it without ever failing.
func TestDegenerateFramesDoNotProduceNaN(t *testing.T) {
	t.Parallel()

	target := coord.NewICRS(angle.Deg(90), angle.Deg(30))

	for _, tc := range []struct {
		name             string
		distance, height float64
	}{
		{"no distance to the center", 0, 20.8},
		{"no distance and no height", 0, 0},
		{"the Sun further from the plane than from the center", 100, 500},
		{"a negative height", 8178, -20.8},
	} {
		got := coord.NewGalactocentricFrame(unit.Pc(tc.distance), unit.Pc(tc.height), vector.Zero()).
			FromICRS(target, unit.Pc(1000))

		if math.IsNaN(got.X().Pc()) || math.IsNaN(got.Y().Pc()) || math.IsNaN(got.Z().Pc()) {
			t.Errorf("%s: produced %s", tc.name, got)
		}
	}
}

// TestParallaxDistanceIsTheReciprocal checks the definition and the unit, the
// latter being the one that goes wrong: a catalogue's milliarcseconds invert to
// kiloparsecs, and the mistake is a factor of a thousand that looks plausible.
func TestParallaxDistanceIsTheReciprocal(t *testing.T) {
	t.Parallel()

	// One arcsecond is one parsec — the definition of the unit.
	testutil.AssertNear(t, "1 arcsec", coord.ParallaxDistance(angle.Arcsec(1)).Pc(), 1, 1e-12)

	// Proxima Centauri: 768.07 mas (Gaia DR3), so a little over 1.3 pc.
	testutil.AssertRelNear(t, "Proxima", coord.ParallaxDistance(angle.Arcsec(0.76807)).Pc(), 1.30197, 1e-5)

	// A milliarcsecond is a kiloparsec, which is the unit trap stated as a test.
	testutil.AssertRelNear(t, "1 mas", coord.ParallaxDistance(angle.Arcsec(0.001)).Pc(), 1000, 1e-12)

	// A zero parallax is an unmeasurably large distance, and says so rather
	// than returning a NaN or an error nobody checks.
	if d := coord.ParallaxDistance(0); !math.IsInf(d.Pc(), 1) {
		t.Errorf("ParallaxDistance(0) = %v, want +Inf", d)
	}

	// A negative parallax is noise, not an error, and inverting it gives a
	// negative number. Documented rather than guarded — see the doc comment.
	if d := coord.ParallaxDistance(angle.Arcsec(-0.001)); d >= 0 {
		t.Errorf("ParallaxDistance(-1 mas) = %v, want a negative number", d)
	}
}

// TestParallaxDistanceFeedsTheFrame is the two pieces used the way the doc
// comments say to use them together, since that is the whole reason
// ParallaxDistance is in this package rather than in a caller.
func TestParallaxDistanceFeedsTheFrame(t *testing.T) {
	t.Parallel()

	// A star toward the Galactic center at 1 mas, so a kiloparsec away — an
	// eighth of the way to the center, and the arithmetic is checkable by eye.
	star := galacticCenter()
	f := coord.DefaultGalactocentricFrame()

	got := f.FromICRS(star, coord.ParallaxDistance(angle.Arcsec(0.001)))

	testutil.AssertRelNear(t, "radius", got.Radius().Pc(), f.SunDistance().Pc()-1000, 1e-3)
}

// TestAPositionEnteredByHandIsIndistinguishableFromAComputedOne exercises
// [coord.NewGalactocentric], which is what makes [coord.GalactocentricFrame.ToICRS]
// reachable at all for a position astrogo did not compute itself.
//
// That is the constructor's whole reason to exist: catalogues of Galactic
// structure publish Cartesian X, Y, Z directly — globular cluster compilations
// and stream tracks among them — and without a way to enter one, the inverse
// transform could only ever be handed output from the forward one.
//
// So the property asserted is that the two origins are interchangeable: a
// position rebuilt from its own components converts back to the same sky
// direction and distance as the original.
func TestAPositionEnteredByHandIsIndistinguishableFromAComputedOne(t *testing.T) {
	t.Parallel()

	f := coord.DefaultGalactocentricFrame()

	star := coord.GalacticToICRS(coord.NewGalactic(angle.Deg(45), angle.Deg(-20)))
	computed := f.FromICRS(star, unit.Pc(3000))

	rebuilt := coord.NewGalactocentric(computed.X(), computed.Y(), computed.Z())

	if rebuilt != computed {
		t.Errorf("rebuilt %s differs from computed %s", rebuilt, computed)
	}

	// And it converts back the same way, which is the use the constructor is
	// for — entering somebody else's X, Y, Z and asking where on the sky it is.
	wantDir, wantDist := f.ToICRS(computed)
	gotDir, gotDist := f.ToICRS(rebuilt)

	if sep := coord.Separation(gotDir, wantDir).Arcseconds(); sep > 1e-9 {
		t.Errorf("direction differs by %.3g arcsec", sep)
	}

	testutil.AssertNear(t, "distance", gotDist.Pc(), wantDist.Pc(), 1e-9)
	testutil.AssertNear(t, "round trip distance", gotDist.Pc(), 3000, 1e-9)
}

// TestTheAccessorsAllReadTheSameVector checks that [coord.Galactocentric.Vector]
// and the three component accessors cannot disagree, and that the two radii are
// derived from that same vector rather than kept alongside it.
//
// Worth asserting because the type stores one vector and exposes five views of
// it: a future change that cached a radius, or that returned a copy from
// Vector while the components read the original, would be invisible to every
// other test here.
func TestTheAccessorsAllReadTheSameVector(t *testing.T) {
	t.Parallel()

	// A deliberately asymmetric position, so a swapped pair of components
	// cannot pass.
	c := coord.NewGalactocentric(unit.Pc(-1234.5), unit.Pc(678.25), unit.Pc(-90.125))

	v := c.Vector()

	testutil.AssertExact(t, "Vector X against X()", v.X, c.X().Meters())
	testutil.AssertExact(t, "Vector Y against Y()", v.Y, c.Y().Meters())
	testutil.AssertExact(t, "Vector Z against Z()", v.Z, c.Z().Meters())

	testutil.AssertExact(t, "Distance against the vector's norm", c.Distance().Meters(), v.Norm())
	testutil.AssertNear(t, "Radius against the in-plane hypotenuse",
		c.Radius().Meters(), math.Hypot(v.X, v.Y), 1e-3)

	// Radius ignores Z and Distance does not, which is the distinction the two
	// doc comments turn on.
	if c.Radius() >= c.Distance() {
		t.Errorf("Radius %.4f is not less than Distance %.4f for a position off the midplane",
			c.Radius().Pc(), c.Distance().Pc())
	}
}

// TestStringNamesItsUnits pins the rendering, and specifically that it says
// "pc".
//
// A bare Cartesian triple with no unit attached is the exact failure this type
// exists to prevent — it is why [coord.GalactocentricFrame.FromICRS] takes its
// distance as a named argument instead of reading [coord.ICRS.Dist]. A String
// that dropped the unit would put that ambiguity straight back into every log
// line and error message.
func TestStringNamesItsUnits(t *testing.T) {
	t.Parallel()

	got := coord.NewGalactocentric(unit.Pc(-8177.9735), 0, unit.Pc(20.8)).String()

	const want = "Galactocentric X -8177.974 Y 0.000 Z 20.800 pc"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}

	// The Sun renders through the same path, so a frame built from the cited
	// parameters is legible without a debugger.
	if sun := coord.DefaultGalactocentricFrame().SunPosition().String(); sun != want {
		t.Errorf("the default frame's Sun renders as %q, want %q", sun, want)
	}
}

// TestAccessorsAreExactlyInvertedByTheConstructor pins the choice of unit for
// the vector [coord.Galactocentric] stores.
//
// A [vector.Vec3] cannot carry a unit, so the one it is in has to be fixed in
// the type. It is meters — the base unit of [unit.Length] — rather than the
// parsecs the frame is read in, which makes [coord.NewGalactocentric] and the
// three component accessors exact inverses of each other. Storing parsecs
// would put a multiply and a divide by 3.086e16 between a component read out
// and the same component entered back in, and the round trip would land a few
// ulp away.
//
// A few ulp is nothing in parsecs and is not nothing for a struct comparison:
// [TestAPositionEnteredByHandIsIndistinguishableFromAComputedOne] asserts
// equality of the whole value, which is the property that makes entering
// somebody else's X, Y, Z equivalent to computing them here.
func TestAccessorsAreExactlyInvertedByTheConstructor(t *testing.T) {
	t.Parallel()

	// Values that are not representable in binary and are far from 1, so a
	// scale factor applied and removed would show.
	for _, tc := range []struct{ x, y, z float64 }{
		{-8177.9735, 0.1, 20.8},
		{1e-3, -1.0 / 3, 1e5},
		{-6187.2013579, 1993.3892468, -1010.3271357},
	} {
		in := coord.NewGalactocentric(unit.Pc(tc.x), unit.Pc(tc.y), unit.Pc(tc.z))

		out := coord.NewGalactocentric(in.X(), in.Y(), in.Z())
		if out != in {
			t.Errorf("(%g, %g, %g) pc: reading the components out and back gave %s, want %s",
				tc.x, tc.y, tc.z, out, in)
		}

		// And the same for the velocity-carrying constructor, which shares
		// the conversion and so could only diverge by being written twice.
		withVel := coord.NewGalactocentricWithVelocity(
			unit.Pc(tc.x), unit.Pc(tc.y), unit.Pc(tc.z), vector.V3(10, 250, 5))

		back := coord.NewGalactocentricWithVelocity(
			withVel.X(), withVel.Y(), withVel.Z(), vector.V3(10, 250, 5))
		if back != withVel {
			t.Errorf("(%g, %g, %g) pc with a velocity: round trip gave %s, want %s",
				tc.x, tc.y, tc.z, back, withVel)
		}
	}
}
