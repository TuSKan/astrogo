package gofaext_test

import (
	"testing"

	"github.com/TuSKan/astrogo/internal/gofaext"
	"github.com/TuSKan/astrogo/internal/testutil"
)

func TestGofaExtWrappers(t *testing.T) {
	// Dtf2d
	d1, d2, st := gofaext.Dtf2d("UTC", 2000, 1, 1, 12, 0, 0.0)
	testutil.AssertEqual(t, "Status", st, 0)
	testutil.AssertNear(t, "D1", d1, 2451544.5, 1e-9)

	// JdToDate
	y, m, d, f, st := gofaext.JdToDate(d1, d2)
	testutil.AssertEqual(t, "Status", st, 0)
	testutil.AssertEqual(t, "Year", y, 2000)
	testutil.AssertEqual(t, "Month", m, 1)
	testutil.AssertEqual(t, "Day", d, 1)
	testutil.AssertNear(t, "Frac", f, 0.5, 1e-9)

	// Seps
	sep := gofaext.Seps(0, 0, 1, 0)
	testutil.AssertNear(t, "Seps", sep, 1.0, 1e-9)

	// Atco13 / Atio13 / Atoc13
	aob, zob, _, _, _, _, st := gofaext.Atco13(0, 0, 0, 0, 0, 0, d1, d2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	testutil.AssertEqual(t, "Status", st, 0)
	gofaext.Atio13(0, 0, d1, d2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	gofaext.Atoc13("R", aob, zob, d1, d2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)

	// Icrs2g / G2icrs
	gl, gb := gofaext.Icrs2g(0, 0)
	ra, _ := gofaext.G2icrs(gl, gb)

	if ra > 3.14 {
		ra -= 2 * 3.14159265358979323846
	} else if ra < -3.14 {
		ra += 2 * 3.14159265358979323846
	}

	testutil.AssertNear(t, "G2icrs RA", ra, 0.0, 1e-9)

	// Eceq06 / Eqec06
	elon, elat := gofaext.Eceq06(d1, d2, 0, 0)
	gofaext.Eqec06(d1, d2, elon, elat)

	// Atic13
	gofaext.Atic13(0, 0, d1, d2)

	// Epv00
	pvh, pvb, st := gofaext.Epv00(d1, d2)
	testutil.AssertEqual(t, "Status", st, 0)

	if pvh[0][0] == 0 && pvb[0][0] == 0 {
		t.Errorf("Epv00 failed, got zero")
	}

	// Moon98
	pv := gofaext.Moon98(d1, d2)
	if pv[0][0] == 0 {
		t.Errorf("Moon98 failed, got zero")
	}

	// Dat
	dat, st := gofaext.Dat(2000, 1, 1, 0.5)
	testutil.AssertEqual(t, "Status", st, 0)

	if dat == 0 {
		t.Errorf("Dat failed, got zero")
	}

	// Gst06a
	gst := gofaext.Gst06a(d1, d2, d1, d2)
	if gst == 0 {
		t.Errorf("Gst06a failed, got zero")
	}
}

// TestC2tcioDecompositionMatchesC2t06a proves that composing the
// decomposed factors (C2i06a, Era00, Pom00/Sp00) via C2tcio reproduces the
// monolithic C2t06a call exactly — the property coord.Context.AtTime relies
// on to cheaply recompute only the Earth-rotation-dependent factor.
func TestC2tcioDecompositionMatchesC2t06a(t *testing.T) {
	const (
		tt1, tt2 = 2451545.0, 0.25 // an arbitrary TT epoch
		ut1, ut2 = 2451545.0, 0.24 // a nearby UT1 epoch (DUT1 offset)
		xp, yp   = 1.5e-7, 2.3e-7  // arbitrary polar motion, radians
	)

	want := gofaext.C2t06a(tt1, tt2, ut1, ut2, xp, yp)

	rc2i := gofaext.C2i06a(tt1, tt2)
	sp := gofaext.Sp00(tt1, tt2)
	rpom := gofaext.Pom00(xp, yp, sp)
	era := gofaext.Era00(ut1, ut2)
	got := gofaext.C2tcio(rc2i, era, rpom)

	for i := range 3 {
		for j := range 3 {
			testutil.AssertNear(t, "C2tcio decomposition", got[i][j], want[i][j], 1e-15)
		}
	}
}

// TestAperUpdatesOnlyEral proves Aper touches only ASTROM.Eral, leaving
// every other field (populated by a real Apco13 call) untouched — the
// property that makes it a safe O(1) substitute for a full Apco13 rebuild
// when only the Earth Rotation Angle has changed.
func TestAperUpdatesOnlyEral(t *testing.T) {
	astrom, _ := gofaext.Apco13(
		2451545.0, 0.5, 0.1,
		0.3, 0.7, 500.0,
		1.5e-7, 2.3e-7,
		1013.25, 15.0, 0.5, 0.55,
	)

	before := astrom
	gofaext.Aper(1.2345, &astrom)

	if astrom.Eral != 1.2345+before.Along {
		t.Errorf("Aper: Eral = %v, want %v", astrom.Eral, 1.2345+before.Along)
	}

	astrom.Eral = before.Eral // neutralize the one expected change

	if astrom != before {
		t.Errorf("Aper mutated a field other than Eral:\nbefore=%+v\nafter=%+v", before, astrom)
	}
}
