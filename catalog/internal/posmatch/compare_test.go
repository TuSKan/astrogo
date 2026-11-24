package posmatch

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/resolve"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/time"
)

// j2016 is Gaia DR3's epoch, J2016.0.
var j2016 = time.FromJD(2457389.0, time.TT)

// mas is v milliarcseconds.
func mas(v float64) angle.Angle { return angle.Arcsec(v / 1000) }

// still is a target with no motion at (10°, decDeg + northMas), at J2000,
// with the errors e.
func still(decDeg, northMas float64, e resolve.AstrometricError) resolve.Target {
	return resolve.Target{
		Coord:    coord.NewICRS(angle.Deg(10), angle.Deg(decDeg).Add(mas(northMas))),
		HasCoord: true, Epoch: time.J2000(), AstrometricError: e,
	}
}

// errorsOf is a position error of sigma mas on each axis, correlated by rho.
func errorsOf(sigma, rho float64) resolve.AstrometricError {
	return resolve.AstrometricError{RA: mas(sigma), Dec: mas(sigma), RADec: rho}
}

// TestCompareWeighsTheOffsetByTheErrors: two positions 3 mas apart, each
// with 1 mas errors, have D² = 3²/(1²+1²) = 4.5 and may be one object
// however tight the threshold; with 0.5 mas errors D² = 18 and they may not,
// however loose it is.
func TestCompareWeighsTheOffsetByTheErrors(t *testing.T) {
	t.Parallel()

	ev, ok, err := Compare(still(0, 0, errorsOf(1, 0)), still(0, 3, errorsOf(1, 0)), mas(1e-6))
	testutil.AssertNoError(t, err)

	if !ev.HasD2 || !ok {
		t.Fatalf("1 mas errors: HasD2 %v, compatible %v; want D² computed and compatible", ev.HasD2, ok)
	}

	testutil.AssertRelNear(t, "D² with 1 mas errors", ev.D2, 4.5, 1e-9)
	testutil.AssertRelNear(t, "separation", ev.Separation.Arcseconds()*1000, 3, 1e-9)

	ev, ok, err = Compare(still(0, 0, errorsOf(0.5, 0)), still(0, 3, errorsOf(0.5, 0)), angle.Arcsec(2))
	testutil.AssertNoError(t, err)

	if !ev.HasD2 || ok {
		t.Fatalf("0.5 mas errors: HasD2 %v, compatible %v; want D² computed and incompatible", ev.HasD2, ok)
	}

	testutil.AssertRelNear(t, "D² with 0.5 mas errors", ev.D2, 18, 1e-9)
}

// TestCompareWeighsTheCorrelation: with 1 mas errors correlated by 0.8 on
// both positions, an offset of 3 mas in α* and 3 mas in δ lies along the
// errors' long axis, D² = 3²/(1+0.8) = 5, and the same offset with δ reversed
// lies across it, D² = 3²/(1−0.8) = 45.
func TestCompareWeighsTheCorrelation(t *testing.T) {
	t.Parallel()

	e := errorsOf(1, 0.8)
	a := still(0, 0, e)

	for _, tc := range []struct {
		name       string
		dec, want  float64
		compatible bool
	}{
		{"along the errors", 3, 5, true},
		{"across the errors", -3, 45, false},
	} {
		// At the equator α* is α, so 3 mas of right ascension is 3 mas east.
		b := resolve.Target{
			Coord:    coord.NewICRS(angle.Deg(10).Add(mas(3)), mas(tc.dec)),
			HasCoord: true, Epoch: time.J2000(), AstrometricError: e,
		}

		ev, ok, err := Compare(a, b, angle.Arcsec(2))
		testutil.AssertNoError(t, err)
		testutil.AssertRelNear(t, tc.name, ev.D2, tc.want, 1e-6)

		if ok != tc.compatible {
			t.Errorf("%s: compatible = %v, want %v", tc.name, ok, tc.compatible)
		}
	}
}

// TestCompareMovesTheErrorsToTheComparisonEpoch: a star given at J2016.0,
// with 0.1 mas position errors, 1 mas/yr motion errors and a 0.5 correlation
// between δ and μδ, is compared with a still position at J2000.0, 16 years
// earlier, where its δ variance is 0.1² + 2·(−16)·0.5·0.1·1 + 16² = 254.41
// mas². A still position 20 mas north of it there has D² = 20²/254.41.
func TestCompareMovesTheErrorsToTheComparisonEpoch(t *testing.T) {
	t.Parallel()

	star := resolve.Target{
		Coord: coord.NewICRS(angle.Deg(10), angle.Deg(20)), HasCoord: true, Epoch: j2016,
		PmDec: mas(100),
		AstrometricError: resolve.AstrometricError{
			RA: mas(0.1), Dec: mas(0.1), PmRA: mas(1), PmDec: mas(1), DecPmDec: 0.5,
		},
	}

	at2000, err := coord.PropagateEpoch(Coord(star), time.J2000())
	testutil.AssertNoError(t, err)

	other := resolve.Target{
		Coord:    coord.NewICRS(at2000.RA(), at2000.Dec().Add(mas(20))),
		HasCoord: true, Epoch: time.J2000(),
		AstrometricError: resolve.AstrometricError{RA: mas(0.001), Dec: mas(0.001)},
	}

	ev, ok, err := Compare(star, other, angle.Arcsec(2))
	testutil.AssertNoError(t, err)

	if !ev.HasD2 || !ok {
		t.Fatalf("HasD2 %v, compatible %v; want D² computed and compatible", ev.HasD2, ok)
	}

	testutil.AssertRelNear(t, "D²", ev.D2, 400/(254.41+1e-6), 1e-6)
}

// TestCovarianceHoldsWhereTheErrorsDo: SIMBAD's position of a star from Gaia
// is at J2000.0 with Gaia's errors at J2016.0. Its covariance is the stated
// one at J2016.0, and at J2000.0 the stated one grown by 16 years of the
// motion errors.
func TestCovarianceHoldsWhereTheErrorsDo(t *testing.T) {
	t.Parallel()

	star := resolve.Target{
		Epoch: time.J2000(), PmDec: mas(100),
		AstrometricError: resolve.AstrometricError{
			RA: mas(0.03), Dec: mas(0.04), PmRA: mas(0.05), PmDec: mas(0.06), Epoch: j2016,
		},
	}

	sq := func(v float64) float64 { return mas(v).Radians() * mas(v).Radians() }

	at2016, ok := covarianceAt(star, j2016)
	if !ok {
		t.Fatal("no covariance at the errors' own epoch")
	}

	testutil.AssertRelNear(t, "σ²(α*) at J2016.0", at2016.xx, sq(0.03), 1e-12)
	testutil.AssertRelNear(t, "σ²(δ) at J2016.0", at2016.yy, sq(0.04), 1e-12)

	at2000, ok := covarianceAt(star, time.J2000())
	if !ok {
		t.Fatal("no covariance at J2000.0")
	}

	testutil.AssertRelNear(t, "σ²(α*) at J2000.0", at2000.xx, sq(0.03)+256*sq(0.05), 1e-9)
	testutil.AssertRelNear(t, "σ²(δ) at J2000.0", at2000.yy, sq(0.04)+256*sq(0.06), 1e-9)
}

// TestCompareFallsBackToTheThreshold: without both position errors, or
// without the motion errors moving one of them needs, D² is not computed and
// the threshold decides.
func TestCompareFallsBackToTheThreshold(t *testing.T) {
	t.Parallel()

	moving := resolve.Target{
		Coord: coord.NewICRS(angle.Deg(10), angle.Deg(20)), HasCoord: true, Epoch: j2016,
		PmDec:            mas(100),
		AstrometricError: resolve.AstrometricError{RA: mas(0.1), Dec: mas(0.1)},
	}

	at2000, err := coord.PropagateEpoch(Coord(moving), time.J2000())
	testutil.AssertNoError(t, err)

	near := resolve.Target{
		Coord:    coord.NewICRS(at2000.RA(), at2000.Dec().Add(mas(500))),
		HasCoord: true, Epoch: time.J2000(),
		AstrometricError: resolve.AstrometricError{RA: mas(0.1), Dec: mas(0.1)},
	}

	bare := near
	bare.AstrometricError = resolve.AstrometricError{}

	for _, tc := range []struct {
		name string
		a, b resolve.Target
	}{
		{"one publishes no position error", still(20, 0, errorsOf(0.1, 0)), still(20, 500, resolve.AstrometricError{})},
		{"moving needs motion errors it does not publish", moving, near},
		{"neither publishes one", moving, bare},
	} {
		for _, threshold := range []float64{0.4, 0.6} {
			ev, ok, err := Compare(tc.a, tc.b, angle.Arcsec(threshold))
			testutil.AssertNoError(t, err)

			if ev.HasD2 {
				t.Errorf("%s: D² computed (%g)", tc.name, ev.D2)
			}

			if want := threshold > 0.5; ok != want {
				t.Errorf("%s, threshold %g″: compatible = %v, want %v (separation %.4f″)",
					tc.name, threshold, ok, want, ev.Separation.Arcseconds())
			}
		}
	}
}

// TestMaxD2IsTheDocumentedLevel: the 99.73% point of a χ² with two degrees
// of freedom, which resolve.Association and catalog/xmatch document as 11.83.
func TestMaxD2IsTheDocumentedLevel(t *testing.T) {
	t.Parallel()

	testutil.AssertExact(t, "maxD2 to two decimals", math.Round(maxD2*100)/100, 11.83)
	testutil.AssertRelNear(t, "1 − exp(−maxD2/2)", 1-math.Exp(-maxD2/2), 0.9973, 1e-12)
}
