package gofaext_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/gofaext"
)

// TestPvstarAgainstSOFAsOwnVector checks the wrapper against the vector SOFA
// publishes for iauPvstar in t_sofa_c.c, reached here through gofa's
// transcription of it.
//
// As with [TestStarpvAgainstSOFAsOwnVector], this tests the wrapper and not the
// astronomy: whether the pv-vector goes in the right way round and the six
// results come back in the right order, which is the only thing a wrapper can
// get wrong and the thing a plausible answer would hide.
func TestPvstarAgainstSOFAsOwnVector(t *testing.T) {
	t.Parallel()

	pv := [2][3]float64{
		{126668.5912743160601, 2136.792716839935195, -245251.2339876830091},
		{-0.4051854035740712739e-2, -0.6253919754866173866e-2, 0.1189353719774107189e-1},
	}

	ra, dec, pmr, pmd, px, rv, status := gofaext.Pvstar(pv)

	if status != 0 {
		t.Errorf("status = %d, want 0", status)
	}

	for _, tc := range []struct {
		name string
		got  float64
		want float64
		tol  float64
	}{
		{"right ascension", ra, 0.1686756e-1, 1e-12},
		{"declination", dec, -1.093989828, 1e-12},
		{"dRA/dt", pmr, -0.1783235160000472788e-4, 1e-16},
		{"dDec/dt", pmd, 0.2336024047000619347e-5, 1e-16},
		{"parallax", px, 0.74723, 1e-12},
		{"radial velocity", rv, -21.60000010107306010, 1e-11},
	} {
		if math.Abs(tc.got-tc.want) > tc.tol {
			t.Errorf("%s = %.17g, want %.17g (tolerance %g)", tc.name, tc.got, tc.want, tc.tol)
		}
	}
}

// TestPvstarReportsTheInputsItCannotUse covers the two negative statuses, which
// the wrapper returns rather than swallowing because both leave every other
// returned value meaningless rather than merely imprecise.
//
// It also pins which is which. The intuitive reading — a missing position being
// the first failure and an impossible speed the second — is backwards: SOFA
// returns −1 for the speed and −2 for the position, because the relativistic
// correction is computed before the direction is extracted. An earlier revision
// of gofaext's wrapper documented them the other way round, and this test is
// what caught it.
func TestPvstarReportsTheInputsItCannotUse(t *testing.T) {
	t.Parallel()

	// Superluminal: one au per day is about 1731 km/s, so 200 au/day is far
	// past c.
	if _, _, _, _, _, _, status := gofaext.Pvstar([2][3]float64{
		{1, 0, 0},
		{200, 0, 0},
	}); status != -1 {
		t.Errorf("a superluminal velocity gave status %d, want -1", status)
	}

	// A zero position vector has no direction to report.
	if _, _, _, _, _, _, status := gofaext.Pvstar([2][3]float64{
		{0, 0, 0},
		{1e-3, 1e-3, 1e-3},
	}); status != -2 {
		t.Errorf("a zero position vector gave status %d, want -2", status)
	}
}

// TestStarpvAndPvstarAreInverses is the property astrogo actually depends on:
// coord.SpaceVelocity uses one and coord.GalactocentricFrame.ToICRS the other,
// so a catalogue entry taken apart and put back together must survive.
func TestStarpvAndPvstarAreInverses(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name                          string
		ra, dec, pmr, pmd, px, rv     float64
		tolAngle, tolRate, tolPx, tol float64
	}{
		{
			name: "SOFA's own case",
			ra:   0.01686756, dec: -1.093989828,
			pmr: -1.78323516e-5, pmd: 2.336024047e-6,
			px: 0.74723, rv: -21.6,
			tolAngle: 1e-12, tolRate: 1e-16, tolPx: 1e-12, tol: 1e-6,
		},
		{
			name: "a fast nearby star",
			ra:   4.7, dec: 0.082,
			pmr: -3.9e-6, pmd: 5.0e-5,
			px: 0.54698, rv: -110.6,
			tolAngle: 1e-12, tolRate: 1e-16, tolPx: 1e-12, tol: 1e-6,
		},
		{
			name: "at rest",
			ra:   2.0, dec: -0.5,
			pmr: 0, pmd: 0,
			px: 0.02, rv: 0,
			tolAngle: 1e-12, tolRate: 1e-18, tolPx: 1e-12, tol: 1e-9,
		},
	} {
		pv, status := gofaext.Starpv(tc.ra, tc.dec, tc.pmr, tc.pmd, tc.px, tc.rv)
		if status != 0 {
			t.Errorf("%s: Starpv status %d", tc.name, status)
			continue
		}

		ra, dec, pmr, pmd, px, rv, status := gofaext.Pvstar(pv)
		if status != 0 {
			t.Errorf("%s: Pvstar status %d", tc.name, status)
			continue
		}

		for _, got := range []struct {
			name       string
			have, want float64
			tol        float64
		}{
			{"ra", ra, tc.ra, tc.tolAngle},
			{"dec", dec, tc.dec, tc.tolAngle},
			{"dRA/dt", pmr, tc.pmr, tc.tolRate},
			{"dDec/dt", pmd, tc.pmd, tc.tolRate},
			{"parallax", px, tc.px, tc.tolPx},
			{"radial velocity", rv, tc.rv, tc.tol},
		} {
			if math.Abs(got.have-got.want) > got.tol {
				t.Errorf("%s: %s round-tripped to %.17g, want %.17g (tolerance %g)",
					tc.name, got.name, got.have, got.want, got.tol)
			}
		}
	}
}
