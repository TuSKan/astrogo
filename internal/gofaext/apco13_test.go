package gofaext_test

import (
	"testing"

	"github.com/hebl/gofa"

	"github.com/TuSKan/astrogo/internal/gofaext"
)

// TestApcoAtIsApco13 holds ApcoAt to the routine it takes apart: given the TT
// and UT1 SOFA's Apco13 derives from a UTC date, ApcoAt's astrometry and
// equation of the origins are Apco13's bit for bit, 1900 to 2100, with polar
// motion, refraction and a nonzero DUT1. All ApcoAt changes is who derives the
// time scales (#474).
func TestApcoAtIsApco13(t *testing.T) {
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

		got, gotEO := gofaext.ApcoAt(tt1, tt2, ut11, ut12, elong, phi, hm, xp, yp, 1000, 10, 0.5, 0.55)

		if got != want || gotEO != wantEO {
			t.Fatalf("UTC %.2f + 0.3: ApcoAt at Apco13's own TT and UT1 is not Apco13", utc1)
		}
	}
}
