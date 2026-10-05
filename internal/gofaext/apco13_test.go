package gofaext_test

import (
	"testing"

	"github.com/TuSKan/astrogo/internal/gofaext"
)

// TestApco13BuildsC2i06aAtUTCToTT is the fact coord.NewContext's reuse rests
// on: the celestial-to-intermediate matrix Apco13 returns, astrom.Bpn, is
// C2i06a's at the TT that UTCToTT derives, bit for bit. Before 1972 included,
// where that TT is not astrogo's own; NewContext guards for that separately
// (#473).
func TestApco13BuildsC2i06aAtUTCToTT(t *testing.T) {
	t.Parallel()

	// 1900 to 2100 in steps of 37.25 days.
	for utc1 := 2415020.5; utc1 < 2488069.5; utc1 += 37.25 {
		astrom, _ := gofaext.Apco13(utc1, 0.3, 0, 0, 0.5, 0, 0, 0, 1000, 10, 0.5, 0.55)

		tt1, tt2 := gofaext.UTCToTT(utc1, 0.3)
		if want := gofaext.C2i06a(tt1, tt2); astrom.Bpn != want {
			t.Fatalf("UTC %.2f + 0.3: Apco13's astrom.Bpn is not C2i06a at UTCToTT", utc1)
		}
	}
}
