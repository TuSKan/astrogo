package coord_test

import (
	"math"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/vector"
)

// astropyLSRD is Astropy's own V_OFFSET_LSRD, the Delhaye solar motion
// expressed in ICRS Cartesian components, in km/s.
//
// It is checked in to Astropy's source as a literal, computed once from
// Galactic (9, 12, 7) — the same published components astrogo starts from,
// rotated by a different implementation. That makes it the one available
// external check on this file: same paper, independent arithmetic.
var astropyLSRD = [3]float64{
	-0.6382306360182073,
	-14.585424483191094,
	7.8011572411006815,
}

// TestLSRDelhayeMatchesAstropysOwnVector compares the rotated solar motion
// against Astropy's, and separates the two things that could differ.
//
// The magnitude must agree to float precision, because both sides start from
// the same published (9, 12, 7) and a rotation does not change a length. Only
// the direction can differ, and it does — by about 10 milliarcseconds, which
// is the gap between the Galactic pole SOFA adopts and the one Astropy does.
// Galactic coordinates are themselves only defined to about a tenth of an
// arcsecond, so that residual is the coordinate system rather than either
// implementation.
//
// Checking the two separately is the point. A single component-wise tolerance
// loose enough to absorb the pole difference would also absorb a transcription
// error in the published components, which is the failure this is here to
// catch.
func TestLSRDelhayeMatchesAstropysOwnVector(t *testing.T) {
	t.Parallel()

	apex, speed := coord.LSRApex(coord.LSRDelhaye)

	got := apex.ToUnitVector().MulScalar(speed)

	wantSpeed := math.Sqrt(9*9 + 12*12 + 7*7)
	astropySpeed := math.Sqrt(astropyLSRD[0]*astropyLSRD[0] +
		astropyLSRD[1]*astropyLSRD[1] + astropyLSRD[2]*astropyLSRD[2])

	testutil.AssertNear(t, "speed (km/s)", speed, wantSpeed, 1e-12)
	testutil.AssertNear(t, "Astropy's own speed (km/s)", astropySpeed, wantSpeed, 1e-12)

	// The angle between the two directions, which is where the whole
	// disagreement lives.
	dot := (got.X*astropyLSRD[0] + got.Y*astropyLSRD[1] + got.Z*astropyLSRD[2]) /
		(speed * astropySpeed)

	sep := math.Acos(math.Min(1, dot)) * 180 / math.Pi * 3600

	t.Logf("astrogo [%.12f %.12f %.12f]", got.X, got.Y, got.Z)
	t.Logf("astropy [%.12f %.12f %.12f]", astropyLSRD[0], astropyLSRD[1], astropyLSRD[2])
	t.Logf("directions differ by %.4f arcsec (%.3f mm/s at this speed)",
		sep, sep/206264.8*speed*1e6)

	if sep > 0.05 {
		t.Errorf("the two directions differ by %.4f arcsec, want under 0.05 — "+
			"the adopted Galactic pole accounts for about 0.01, and anything "+
			"much larger is a rotation applied wrongly rather than a definition "+
			"difference", sep)
	}
}

// TestLSRApexMatchesTheLiterature checks the entered components against the
// form their own authors state them in, which is a different statement in the
// same paper rather than a restatement of the numbers themselves.
//
// Delhaye gives the apex as Galactic l = 53°, b = 25°; Schönrich, Binney &
// Dehnen give a total solar motion of 18.0 km/s. Either would catch a
// transposed or mistyped component, which is the realistic failure for a
// hand-entered triple.
func TestLSRApexMatchesTheLiterature(t *testing.T) {
	t.Parallel()

	apex, speed := coord.LSRApex(coord.LSRDelhaye)
	gal := coord.ICRSToGalactic(apex)

	t.Logf("Delhaye apex: galactic l %.4f b %.4f, speed %.4f km/s",
		gal.L().Degrees(), gal.B().Degrees(), speed)

	// The published figures are given to the degree, so the tolerance is the
	// rounding and not a measurement.
	testutil.AssertNear(t, "Delhaye apex l (deg)", gal.L().Degrees(), 53, 0.5)
	testutil.AssertNear(t, "Delhaye apex b (deg)", gal.B().Degrees(), 25, 0.5)

	_, dynamicalSpeed := coord.LSRApex(coord.LSRDynamical)

	testutil.AssertNear(t, "Schönrich+ total solar motion (km/s)", dynamicalSpeed, 18.0, 0.05)
}

// TestLSRCorrectionIsTheProjection pins the three values a projection has to
// take, at the places where they are not a matter of opinion.
func TestLSRCorrectionIsTheProjection(t *testing.T) {
	t.Parallel()

	for _, kind := range []coord.LSRKind{coord.LSRDynamical, coord.LSRDelhaye} {
		t.Run(kind.String(), func(t *testing.T) {
			t.Parallel()

			apex, speed := coord.LSRApex(kind)

			// At the apex the Sun is moving straight at the target, so the
			// correction is the whole solar motion and positive.
			testutil.AssertNear(t, "at the apex", coord.LSRCorrection(apex, kind), speed, 1e-12)

			// At the antapex it is the whole thing, negated.
			antapex := coord.NewICRS(apex.RA().Add(angle.Deg(180)).Wrap360(), apex.Dec().MulScalar(-1))

			testutil.AssertNear(t, "at the antapex", coord.LSRCorrection(antapex, kind), -speed, 1e-12)

			// And 90° away there is no radial component at all. Built as a
			// cross product with the apex rather than written down, so it is
			// perpendicular by construction wherever the apex happens to be.
			a := apex.ToUnitVector()

			ref := vector.V3(0, 0, 1)
			if math.Abs(a.Z) > 0.9 {
				ref = vector.V3(1, 0, 0)
			}

			var side coord.ICRS
			side.FromUnitVector(a.Cross(ref).Unit())

			testutil.AssertNear(t, "90 degrees from the apex", coord.LSRCorrection(side, kind), 0, 1e-12)
		})
	}
}

// TestLSRCorrectionNeverExceedsTheSolarMotion sweeps the sky, because a
// projection that is right at three points and wrong in between would pass
// [TestLSRCorrectionIsTheProjection].
func TestLSRCorrectionNeverExceedsTheSolarMotion(t *testing.T) {
	t.Parallel()

	_, speed := coord.LSRApex(coord.LSRDynamical)

	var maxSeen float64

	for ra := 0.0; ra < 360; ra += 3 {
		for dec := -89.0; dec <= 89; dec += 3 {
			corr := coord.LSRCorrection(
				coord.NewICRS(angle.Deg(ra), angle.Deg(dec)), coord.LSRDynamical)

			if math.Abs(corr) > speed+1e-12 {
				t.Fatalf("correction %g km/s at RA %g Dec %g exceeds the solar motion %g",
					corr, ra, dec, speed)
			}

			maxSeen = math.Max(maxSeen, math.Abs(corr))
		}
	}

	// The sweep has to come close to the full value somewhere, or it is
	// wandering over a sky the apex is not in.
	if maxSeen < 0.99*speed {
		t.Errorf("the largest correction found over the whole sky was %g km/s, "+
			"against a solar motion of %g — the apex was missed", maxSeen, speed)
	}
}

// TestLSRKindsDisagreeByTwoKilometresPerSecond is why the convention is named
// at the call site instead of being chosen here.
//
// The two solar motions differ by (11.1, 12.24, 7.25) − (9, 12, 7) =
// (2.1, 0.24, 0.25) km/s, whose length is 2.13 km/s. That is the largest the
// two conventions can ever disagree for one target, and it is far above the
// precision of any radial velocity worth correcting — so quoting a v_LSR
// without saying which LSR is quoting a number with a 2 km/s ambiguity in it.
func TestLSRKindsDisagreeByTwoKilometresPerSecond(t *testing.T) {
	t.Parallel()

	want := math.Sqrt(2.1*2.1 + 0.24*0.24 + 0.25*0.25)

	var maxDiff float64

	for ra := 0.0; ra < 360; ra += 2 {
		for dec := -89.0; dec <= 89; dec += 2 {
			c := coord.NewICRS(angle.Deg(ra), angle.Deg(dec))

			diff := math.Abs(coord.LSRCorrection(c, coord.LSRDynamical) -
				coord.LSRCorrection(c, coord.LSRDelhaye))

			maxDiff = math.Max(maxDiff, diff)
		}
	}

	t.Logf("the two conventions differ by up to %.4f km/s (predicted %.4f)", maxDiff, want)

	if math.Abs(maxDiff-want) > 0.01 {
		t.Errorf("largest disagreement over the sky is %g km/s, want %g — the "+
			"difference of the two published solar motions", maxDiff, want)
	}
}

// TestLSRKindNamesItsSource keeps the convention identifiable in a log line or
// a report, since the number alone does not say which frame it is in.
func TestLSRKindNamesItsSource(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		kind coord.LSRKind
		want string
	}{
		{coord.LSRDynamical, "Schönrich"},
		{coord.LSRDelhaye, "Delhaye"},
	} {
		if s := tc.kind.String(); !strings.Contains(s, tc.want) {
			t.Errorf("String() = %q, want it to name %q", s, tc.want)
		}
	}

	if s := coord.LSRKind(42).String(); !strings.Contains(s, "42") {
		t.Errorf("an unknown kind renders as %q, want it to show the value", s)
	}
}

// TestUnknownLSRKindFallsBackRatherThanReturningZero pins the one branch with
// no published value behind it.
//
// An [coord.LSRKind] this package does not know is a caller's mistake and there
// is no error return to report it through, so the choice is between a wrong
// number and a *plausible* wrong number. Zero would be the plausible one: a
// correction of zero reads as "none needed" and would be silently believed,
// which is worse than answering with the modern default and saying so in the
// kind's own String.
func TestUnknownLSRKindFallsBackRatherThanReturningZero(t *testing.T) {
	t.Parallel()

	target := coord.NewICRS(angle.Deg(83.8221), angle.Deg(-5.3911))

	got := coord.LSRCorrection(target, coord.LSRKind(42))
	want := coord.LSRCorrection(target, coord.LSRDynamical)

	if got == 0 {
		t.Fatal("an unknown kind produced a zero correction, which a caller " +
			"cannot distinguish from a target genuinely perpendicular to the " +
			"solar motion")
	}

	testutil.AssertNear(t, "unknown kind", got, want, 1e-12)
}

func BenchmarkLSRCorrection(b *testing.B) {
	c := coord.NewICRS(angle.Deg(83.8221), angle.Deg(-5.3911))

	b.ReportAllocs()

	for range b.N {
		_ = coord.LSRCorrection(c, coord.LSRDynamical)
	}
}

// TestLSRKMatchesAstropysRealisation is the check that makes the derived
// kinematic apex trustworthy, and the record of what it costs.
//
// Gordon (1975) publishes LSRK as an apex referred to the B1900 equinox.
// astrogo derives the ICRS vector from that statement; Astropy derives it from
// the same statement through a full FK4-with-equinox implementation and checks
// the result in as a literal. Two independent realisations of one definition,
// so agreement is evidence and disagreement is a bug in one of them.
//
// They do not agree exactly, and the residual is explained rather than
// tolerated. astrogo has no Newcomb precession — SOFA does not provide one —
// so it precesses B1900 to B1950 with the IAU 1976 model. The two differ by
// about 1.1 arcsec per century in the precession constant, and half a century
// of that is what the test measures.
func TestLSRKMatchesAstropysRealisation(t *testing.T) {
	t.Parallel()

	// astropy.coordinates.builtin_frames.galactic_transforms, V_OFFSET_LSRK.
	astropy := vector.Vec3{
		X: 0.28999706839034606,
		Y: -17.317264789717928,
		Z: 10.00141199546947,
	}

	apex, speed := coord.LSRApex(coord.LSRKinematic)
	got := apex.ToUnitVector().MulScalar(speed)

	diff := math.Sqrt(
		(got.X-astropy.X)*(got.X-astropy.X) +
			(got.Y-astropy.Y)*(got.Y-astropy.Y) +
			(got.Z-astropy.Z)*(got.Z-astropy.Z))

	sepArcsec := 2 * math.Asin(diff/2/speed) * 206264.806

	t.Logf("astrogo [%+.15f, %+.15f, %+.15f] km/s", got.X, got.Y, got.Z)
	t.Logf("astropy [%+.15f, %+.15f, %+.15f] km/s", astropy.X, astropy.Y, astropy.Z)
	t.Logf("difference %.3e km/s (%.1f cm/s), direction %.4f arcsec",
		diff, diff*100000, sepArcsec)

	// The speed is Gordon's own number and is not approximated by anything, so
	// it must be exact to float precision.
	if math.Abs(speed-20.0) > 1e-12 {
		t.Errorf("apex speed is %.15f km/s, want exactly 20", speed)
	}

	// A quarter of an arcsecond either side of the 0.55 the model difference
	// predicts. Tight enough that a wrong rotation, a wrong equinox or a
	// dropped E-term shows immediately — each of those is degrees or tens of
	// arcseconds — and loose enough not to trip on the residual itself.
	const wantArcsec, tolArcsec = 0.55, 0.25

	if math.Abs(sepArcsec-wantArcsec) > tolArcsec {
		t.Errorf("astrogo and astropy differ by %.4f arcsec, want %.2f ± %.2f — "+
			"the residual should be half a century of the Newcomb/IAU-1976 precession "+
			"constant difference and nothing else",
			sepArcsec, wantArcsec, tolArcsec)
	}

	// And the velocity error that residual amounts to, which is the number a
	// caller actually cares about.
	if diff > 1e-4 {
		t.Errorf("the apex differs from Astropy's by %.3e km/s, want under 1e-4", diff)
	}
}

// TestLSRKinematicIsDistinctFromTheDynamicalOnes guards against the kind
// silently falling through to a dynamical answer, which is what the default
// branch of solarMotion does for an unknown value.
func TestLSRKinematicIsDistinctFromTheDynamicalOnes(t *testing.T) {
	t.Parallel()

	kinematic, _ := coord.LSRApex(coord.LSRKinematic)
	dynamical, _ := coord.LSRApex(coord.LSRDynamical)
	delhaye, _ := coord.LSRApex(coord.LSRDelhaye)

	for _, other := range []struct {
		name string
		apex coord.ICRS
	}{
		{"LSRD (Schönrich 2010)", dynamical},
		{"LSRD (Delhaye 1965)", delhaye},
	} {
		sep := coord.Separation(kinematic, other.apex).Degrees()

		t.Logf("LSRK is %.1f° from %s", sep, other.name)

		// The kinematic and dynamical standards genuinely point tens of degrees
		// apart; anything under a degree would mean the switch fell through.
		if sep < 1 {
			t.Errorf("LSRK is only %.4f° from %s, which means it is not being "+
				"computed separately", sep, other.name)
		}
	}

	if got := coord.LSRKinematic.String(); got != "LSRK (Gordon 1975)" {
		t.Errorf("String() = %q, want it to name the kind and its source", got)
	}
}
