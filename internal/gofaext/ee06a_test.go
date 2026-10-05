package gofaext_test

import (
	"testing"

	"github.com/TuSKan/astrogo/internal/gofaext"
)

// TestEe06aFromBPNIsEe06a holds the satellite path's equation of the
// equinoxes to the routine it replaces: given Pnm06a's matrix at the same TT,
// it is Ee06a bit for bit, 1900 to 2100 (#476).
func TestEe06aFromBPNIsEe06a(t *testing.T) {
	t.Parallel()

	for tt1 := 2415020.5; tt1 < 2488069.5; tt1 += 37.25 {
		rnpb := gofaext.Pnm06a(tt1, 0.3)

		got := gofaext.Ee06aFromBPN(tt1, 0.3, rnpb)
		if want := gofaext.Ee06a(tt1, 0.3); got != want {
			t.Fatalf("TT %.2f + 0.3: Ee06aFromBPN = %.17g, Ee06a = %.17g", tt1, got, want)
		}
	}
}
