//go:build validation || network

package jpl_test

import (
	"github.com/TuSKan/astrogo/time"
	"testing"
)

// settledMargin is how far behind the present a corpus epoch must sit.
//
// IERS Earth-orientation values reach final status roughly a year after the
// fact; before that they are rapid-service or predicted, and they move. A
// year and a half leaves room for that without demanding the corpus be
// rebuilt to stay valid.
const settledMargin = 18 * 30 * 24 * time.Hour

// TestCorpusEpochsAreSettled fails if the corpus samples an epoch whose
// reference value is still moving.
//
// The regular span used to run to 2027-01-01, which put its last epoch in the
// future. Horizons' answer there depends on predicted Earth orientation, so
// it changed between generation and the next run — by up to 8.4e-05 degrees —
// and TestGenerateCorpus reported a dirty diff every time thereafter, for a
// corpus nobody had touched.
//
// The failing test is the smaller half of the problem. A frozen entry whose
// true value is still moving is a reference that goes quietly out of date:
// the consumer compares astrogo, using today's IERS series, against a
// prediction made on the day the corpus was written, and the two drift apart
// for reasons that have nothing to do with astrogo. A tolerance wide enough
// to absorb that is a tolerance wide enough to hide a regression.
//
// This is checked rather than left to the span comments because the spans are
// written as date strings, where "2027" looks no different from "2025".
func TestCorpusEpochsAreSettled(t *testing.T) {
	c, err := loadCorpus()
	if err != nil {
		t.Skipf("corpus unavailable: %v", err)
	}

	if len(c.Entries) == 0 {
		t.Fatal("corpus has no entries")
	}

	// Unix epoch as a Julian Date, for converting the corpus' own column.
	const (
		unixEpochJD = 2440587.5
		secondsPerD = 86400
	)

	cutoff := time.Now().UTC().Add(-settledMargin)

	var (
		latest  time.GoTime
		offense string
		count   int
	)

	for _, e := range c.Entries {
		when := time.Unix(int64((e.EpochJDUT-unixEpochJD)*secondsPerD), 0).UTC()

		if when.After(latest) {
			latest = when
		}

		if when.After(cutoff) {
			count++

			if offense == "" {
				offense = e.key()
			}
		}
	}

	if count > 0 {
		t.Errorf("%d corpus entries are inside the unsettled window (after %s); first is %s.\n"+
			"Horizons' answer at such an epoch depends on predicted Earth orientation and "+
			"moves after generation. Move the span in corpusSpans further into the past.",
			count, cutoff.Format(time.DateOnly), offense)
	}

	t.Logf("latest corpus epoch %s, cutoff %s", latest.Format(time.DateOnly), cutoff.Format(time.DateOnly))
}
