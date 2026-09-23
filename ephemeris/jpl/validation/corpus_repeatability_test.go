//go:build network && validation

package jpl_test

import (
	"math"
	"strconv"
	"testing"
)

// repeatBody and repeatSite name one query the corpus diff reports as moved.
//
// Jupiter at the synthetic equator, over the regular span. Chosen because it is
// where the diff lands hardest — both of the kinds of change it reports, an
// elevation moving by 1e-06 degrees and a range moving by ~1e-14 AU, appear in
// this one series.
const (
	repeatBody   = 599
	repeatName   = "Jupiter"
	repeatSite   = "Equator (synthetic, 0N 0E)"
	repeatStart  = "2021-01-01 00:00"
	repeatStop   = "2025-01-01 00:00"
	repeatStep   = "120d"
	repeatLonDeg = 0.0
	repeatLatDeg = 0.0
	repeatHeight = 0.0
)

// TestHorizonsAnswersTheSameQueryTheSameWay asks whether Horizons is
// deterministic, which is the question #352 turns on and the one the corpus
// diff cannot answer on its own.
//
// # What is being decided
//
// TestGenerateCorpus reports 94 of 300 entries moved since the corpus was
// written, by 1e-06 degrees in the angles and ~1e-14 AU in range. Three
// explanations fit that shape equally well from the diff alone:
//
//  1. Horizons changed its ephemeris or its observer reduction. Then the corpus
//     is stale and -update-corpus is correct.
//  2. Horizons emits the same computation with different rounding or precision
//     from one call to the next. Then the corpus must not chase it, because the
//     next run would move again.
//  3. astrogo changed what it asks. Ruled out separately: the query set is
//     identical (0 added, 0 removed), and the request builders in
//     horizons_api_test.go are untouched since the commit the manifest records.
//
// Asking twice separates (1) from (2), and nothing else does. Two answers that
// disagree with each other are emission noise whatever they say about the
// corpus; two that agree with each other and differ from the corpus are a real
// upstream change.
//
// # Why this stays in the repository
//
// Because the question recurs. Every future corpus diff needs it answered
// before -update-corpus is run, and the answer is not derivable from the diff —
// it needs a second fetch, which is a thing to run rather than a thing to
// reason about. It also guards the assumption the corpus rests on: that a
// Horizons answer for a settled epoch is a fixed quantity. If that stops being
// true, a frozen reference corpus is the wrong instrument and this is where it
// says so.
//
// Two fetches back to back rather than minutes apart. If Horizons is serving
// from a cache keyed on the query, a gap would be the more searching test — but
// a difference found back to back is conclusive on its own, and an agreement
// found back to back is what the corpus needs in order to be worth updating.
func TestHorizonsAnswersTheSameQueryTheSameWay(t *testing.T) {
	requireHorizons(t)

	first, err := fetchObserverSeries(strconv.Itoa(repeatBody), repeatName,
		repeatLonDeg, repeatLatDeg, repeatHeight, repeatStart, repeatStop, repeatStep)
	if err != nil {
		skipIfHorizonsDown(t, err)
		t.Fatalf("first observer series for %s: %v", repeatName, err)
	}

	second, err := fetchObserverSeries(strconv.Itoa(repeatBody), repeatName,
		repeatLonDeg, repeatLatDeg, repeatHeight, repeatStart, repeatStop, repeatStep)
	if err != nil {
		skipIfHorizonsDown(t, err)
		t.Fatalf("second observer series for %s: %v", repeatName, err)
	}

	if len(first) != len(second) {
		t.Fatalf("the same query returned %d rows and then %d", len(first), len(second))
	}

	if len(first) == 0 {
		t.Fatal("the query returned no rows; this test cannot decide anything")
	}

	// Exact equality, deliberately. The question is not whether the two answers
	// are close — everything in this diff is close — but whether they are the
	// same bits. A tolerance here would answer a question nobody asked.
	var moved int

	for i := range first {
		a, b := first[i], second[i]

		for _, f := range []struct {
			name string
			a, b float64
		}{
			{"astrometric RA", a.AstroRA, b.AstroRA},
			{"astrometric Dec", a.AstroDec, b.AstroDec},
			{"apparent RA", a.AppRA, b.AppRA},
			{"apparent Dec", a.AppDec, b.AppDec},
			{"azimuth", a.Azimuth, b.Azimuth},
			{"elevation", a.Elevation, b.Elevation},
			{"range", a.Range, b.Range},
		} {
			if f.a != f.b {
				moved++

				t.Errorf("row %d %s: %.17g then %.17g (by %.3g)",
					i, f.name, f.a, f.b, math.Abs(f.a-f.b))
			}
		}
	}

	if moved > 0 {
		t.Logf("Horizons gave two different answers to one query in %d fields. "+
			"The corpus must not be updated to either of them: whatever is checked in "+
			"would move again on the next run. See #352.", moved)

		return
	}

	t.Logf("Horizons answered identically twice for %s at %s, across %d rows and 7 "+
		"fields each. A corpus diff therefore reflects a real upstream change rather "+
		"than emission noise, and -update-corpus is the right response to one. See #352.",
		repeatName, repeatSite, len(first))
}
