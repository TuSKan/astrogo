package gofaext_test

import (
	"testing"

	"github.com/hebl/gofa"

	"github.com/TuSKan/astrogo/internal/gofaext"
)

// TestCelestialAtAndApcoFromAreApco13 holds the two halves to the routine
// they take apart: given the TT and UT1 SOFA's Apco13 derives from a UTC
// date, CelestialAt then ApcoFrom give Apco13's astrometry and equation of
// the origins bit for bit, 1900 to 2100, with polar motion, refraction and a
// nonzero DUT1. All the split changes is who derives the time scales (#474),
// and that the series can be evaluated apart from Earth rotation (#790).
func TestCelestialAtAndApcoFromAreApco13(t *testing.T) {
	t.Parallel()

	const (
		elong, phi, hm = -1.2287, -0.4297, 2635
		xp, yp, dut1   = 1.1e-6, 1.7e-6, 0.12
	)

	for utc1 := 2415020.5; utc1 < 2488069.5; utc1 += 37.25 {
		var (
			want   gofa.ASTROM
			wantEO float64
		)

		gofa.Apco13(utc1, 0.3, dut1, elong, phi, hm, xp, yp, 1000, 10, 0.5, 0.55, &want, &wantEO)

		var tai1, tai2, tt1, tt2, ut11, ut12 float64

		gofa.Utctai(utc1, 0.3, &tai1, &tai2)
		gofa.Taitt(tai1, tai2, &tt1, &tt2)
		gofa.Utcut1(utc1, 0.3, dut1, &ut11, &ut12)

		cel := gofaext.CelestialAt(tt1, tt2)
		refa, refb := gofaext.Refco(1000, 10, 0.5, 0.55)
		got := gofaext.ApcoFrom(&cel, tt1, tt2, gofaext.Era00(ut11, ut12), elong, phi, hm, xp, yp, refa, refb)
		gotEO := cel.EO

		if got != want || gotEO != wantEO {
			t.Fatalf("UTC %.2f + 0.3: CelestialAt and ApcoFrom at Apco13's own TT and UT1 are not Apco13", utc1)
		}
	}
}
