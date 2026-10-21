package gofaext_test

import (
	"math"
	"testing"

	"github.com/hebl/gofa"

	"github.com/TuSKan/astrogo/internal/gofaext"
)

// TestMeanEquinoxFromCIOIsGmst06LessEra holds the polynomial to the routine it
// is taken from: GMST minus the Earth rotation angle, isolated as iauEe06a
// isolates it, at UT1 = 0 so the rotation angles cancel. Not bit for bit,
// since that route adds and removes a full turn and normalizes twice; to
// rounding of an angle near 2π, 1900 to 2100.
func TestMeanEquinoxFromCIOIsGmst06LessEra(t *testing.T) {
	t.Parallel()

	// A few ulps of 2π, which is what the normalizations cost: 2e-9″.
	const tol = 1e-14

	for tt1 := 2415020.5; tt1 < 2488069.5; tt1 += 37.25 {
		want := gofa.Anpm(gofa.Gmst06(0, 0, tt1, 0.3) - gofa.Era00(0, 0))

		if got := gofaext.MeanEquinoxFromCIO(tt1, 0.3); math.Abs(got-want) > tol {
			t.Fatalf("TT %.2f + 0.3: MeanEquinoxFromCIO = %.17g, Gmst06 − Era00 = %.17g", tt1, got, want)
		}
	}
}
