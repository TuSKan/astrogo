package gofaext_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/gofaext"
)

// The reference values below are SOFA's own, from its validation program
// t_sofa_c.c, reached here through gofa's transcription of it. They are the
// right oracle for these wrappers precisely because they test nothing about
// astronomy: what can go wrong in a wrapper is the argument order and which
// pointer receives which output, and a routine handed its six inputs in the
// wrong order still returns six plausible numbers.
//
// Tolerances are SOFA's own too, loosened by two orders of magnitude in the
// last place where a Go build's FMA contraction can differ from the C one.
// They are far tighter than any confusion of arguments could survive.

func near(t *testing.T, name string, got, want, tol float64) {
	t.Helper()

	if math.Abs(got-want) > tol {
		t.Errorf("%s = %.17g, want %.17g (tolerance %g, off by %g)", name, got, want, tol, got-want)
	}
}

// TestFk425 checks the full six-element FK4 B1950 to FK5 J2000 conversion.
func TestFk425(t *testing.T) {
	t.Parallel()

	r, d, dr, dd, p, v := gofaext.Fk425(
		0.07626899753879587532, -1.137405378399605780,
		0.1973749217849087460e-4, 0.5659714913272723189e-5,
		0.134, 8.7,
	)

	near(t, "r2000", r, 0.08757989933556446040, 1e-12)
	near(t, "d2000", d, -1.132279113042091895, 1e-10)
	near(t, "dr2000", dr, 0.1953670614474396139e-4, 1e-15)
	near(t, "dd2000", dd, 0.5637686678659640164e-5, 1e-16)
	near(t, "p2000", p, 0.1339919950582767871, 1e-11)
	near(t, "v2000", v, 8.736999669183529069, 1e-10)
}

// TestFk524 checks the inverse, FK5 J2000 to FK4 B1950.
func TestFk524(t *testing.T) {
	t.Parallel()

	r, d, dr, dd, p, v := gofaext.Fk524(
		0.8723503576487275595, -0.7517076365138887672,
		0.2019447755430472323e-4, 0.3541563940505160433e-5,
		0.1559, 86.87,
	)

	near(t, "r1950", r, 0.8636359659799603487, 1e-11)
	near(t, "d1950", d, -0.7550281733160843059, 1e-11)
	near(t, "dr1950", dr, 0.2023628192747172486e-4, 1e-15)
	near(t, "dd1950", dd, 0.3624459754935334718e-5, 1e-16)
	near(t, "p1950", p, 0.1560079963299390241, 1e-11)
	near(t, "v1950", v, 86.79606353469163751, 1e-9)
}

// TestFk45z checks the position-only FK4 to FK5 conversion, the one that
// supplies the fictitious proper motion FK4's drifting equinox implies.
func TestFk45z(t *testing.T) {
	t.Parallel()

	r, d := gofaext.Fk45z(0.01602284975382960982, -0.1164347929099906024, 1954.677617625256806)

	near(t, "r2000", r, 0.02719295911606862303, 1e-13)
	near(t, "d2000", d, -0.1115766001565926892, 1e-11)
}

// TestFk54z checks the inverse, and in particular that the two proper-motion
// outputs arrive in the right order: they differ by less than a factor of two
// here, so swapping them survives any check that only looks at magnitude.
func TestFk54z(t *testing.T) {
	t.Parallel()

	r, d, dr, dd := gofaext.Fk54z(0.02719026625066316119, -0.1115815170738754813, 1954.677308160316374)

	near(t, "r1950", r, 0.01602015588390065476, 1e-12)
	near(t, "d1950", d, -0.1164397101110765346, 1e-11)
	near(t, "dr1950", dr, -0.1175712648471090704e-7, 1e-18)
	near(t, "dd1950", dd, 0.2108109051316431056e-7, 1e-18)
}

// TestFk52h checks FK5 J2000 to the Hipparcos frame — the ICRS realisation
// the FK4 conversions route through.
func TestFk52h(t *testing.T) {
	t.Parallel()

	r, d, dr, dd, px, rv := gofaext.Fk52h(
		1.76779433, -0.2917517103,
		-1.91851572e-7, -5.8468475e-6,
		0.379210, -7.6,
	)

	near(t, "rh", r, 1.767794226299947632, 1e-12)
	near(t, "dh", d, -0.2917516070530391757, 1e-12)
	near(t, "drh", dr, -0.1961874125605721270e-6, 1e-17)
	near(t, "ddh", dd, -0.58459905176693911e-5, 1e-17)
	near(t, "pxh", px, 0.37921, 1e-12)
	near(t, "rvh", rv, -7.6000000940000254, 1e-9)
}

// TestH2fk5 checks the inverse, Hipparcos to FK5 J2000.
func TestH2fk5(t *testing.T) {
	t.Parallel()

	r, d, dr, dd, px, rv := gofaext.H2fk5(
		1.767794352, -0.2917512594,
		-2.76413026e-6, -5.92994449e-6,
		0.379210, -7.6,
	)

	near(t, "r5", r, 1.767794455700065506, 1e-11)
	near(t, "d5", d, -0.2917513626469638890, 1e-11)
	near(t, "dr5", dr, -0.27597945024511204e-5, 1e-16)
	near(t, "dd5", dd, -0.59308014093262838e-5, 1e-16)
	near(t, "px5", px, 0.37921, 1e-11)
	near(t, "rv5", rv, -7.6000001309071126, 1e-9)
}

// TestFk5hz checks the position-only FK5 to Hipparcos rotation, which depends
// on the epoch because the two frames also differ by a spin.
func TestFk5hz(t *testing.T) {
	t.Parallel()

	r, d := gofaext.Fk5hz(1.76779433, -0.2917517103, 2400000.5, 54479.0)

	near(t, "rh", r, 1.767794191464423978, 1e-10)
	near(t, "dh", d, -0.2917516001679884419, 1e-10)
}

// TestHfk5z checks the inverse, including the proper motion the FK5 position
// acquires from that spin.
func TestHfk5z(t *testing.T) {
	t.Parallel()

	r, d, dr, dd := gofaext.Hfk5z(1.767794352, -0.2917512594, 2400000.5, 54479.0)

	near(t, "r5", r, 1.767794490535581026, 1e-11)
	near(t, "d5", d, -0.2917513695320114258, 1e-12)
	near(t, "dr5", dr, 0.4335890983539243029e-8, 1e-20)
	near(t, "dd5", dd, -0.8569648841237745902e-9, 1e-21)
}
