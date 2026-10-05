package gofaext_test

import (
	"testing"

	"github.com/TuSKan/astrogo/internal/gofaext"
)

// counted returns how many precession-nutation series evaluations f records.
func counted(f func()) int64 {
	before := gofaext.SeriesEvaluations()

	f()

	return gofaext.SeriesEvaluations() - before
}

// TestEverySeriesWrapperCountsOnce holds the counter to the wrappers: each one
// whose SOFA routine reaches Nut00a records exactly one evaluation, and the
// wrappers around cheap routines record none. A wrapper added later that
// evaluates the series without counting would let a work contract pass while
// the work it guards doubled.
//
// Not parallel: the counter is process-wide.
func TestEverySeriesWrapperCountsOnce(t *testing.T) {
	const tt1, tt2 = 2460000.5, 0.25

	for _, c := range []struct {
		name string
		call func()
		want int64
	}{
		{"Nut06a", func() { gofaext.Nut06a(tt1, tt2) }, 1},
		{"Pnm06a", func() { gofaext.Pnm06a(tt1, tt2) }, 1},
		{"C2i06a", func() { gofaext.C2i06a(tt1, tt2) }, 1},
		{"C2t06a", func() { gofaext.C2t06a(tt1, tt2, tt1, tt2, 0, 0) }, 1},
		{"Gst06a", func() { gofaext.Gst06a(tt1, tt2, tt1, tt2) }, 1},
		{"Ee06a", func() { gofaext.Ee06a(tt1, tt2) }, 1},
		{"Apco13", func() { gofaext.Apco13(tt1, tt2, 0, 0, 0.5, 0, 0, 0, 1000, 10, 0.5, 0.55) }, 1},
		{"Atco13", func() {
			gofaext.Atco13(1, 0.5, 0, 0, 0, 0, tt1, tt2, 0, 0, 0.5, 0, 0, 0, 1000, 10, 0.5, 0.55)
		}, 1},
		{"Atoc13", func() { gofaext.Atoc13("R", 1, 0.5, tt1, tt2, 0, 0, 0.5, 0, 0, 0, 1000, 10, 0.5, 0.55) }, 1},
		{"Atci13", func() { gofaext.Atci13(1, 0.5, 0, 0, 0, 0, tt1, tt2) }, 1},
		{"Atic13", func() { gofaext.Atic13(1, 0.5, tt1, tt2) }, 1},

		// Cheap routines, beside them in NewContext and AtTime, which must not
		// count: CIRS to observed, Earth rotation, polar motion, the TT this
		// file adds.
		{"Atio13", func() { gofaext.Atio13(1, 0.5, tt1, tt2, 0, 0, 0.5, 0, 0, 0, 1000, 10, 0.5, 0.55) }, 0},
		{"Era00", func() { _ = gofaext.Era00(tt1, tt2) }, 0},
		{"Sp00", func() { _ = gofaext.Sp00(tt1, tt2) }, 0},
		{"Pom00", func() { _ = gofaext.Pom00(0, 0, 0) }, 0},
		{"UTCToTT", func() { _, _ = gofaext.UTCToTT(tt1, tt2) }, 0},
	} {
		if got := counted(c.call); got != c.want {
			t.Errorf("%s recorded %d evaluations of the precession-nutation series, want %d", c.name, got, c.want)
		}
	}
}

// TestApco13BuildsC2i06aAtUTCToTT is the fact coord.NewContext's reuse rests
// on: the celestial-to-intermediate matrix Apco13 returns, astrom.Bpn, is
// C2i06a's at the TT that UTCToTT derives, bit for bit. Before 1972 included,
// where that TT is not astrogo's own; NewContext guards for that separately.
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
