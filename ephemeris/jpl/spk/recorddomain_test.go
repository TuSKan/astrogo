package spk_test

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
)

// TestChebyshevRecordRefusesEpochsPastItsInterval: an epoch after a
// segment's final instant is refused, not evaluated on the last record's
// polynomial outside the interval it fits (#815). The final instant itself
// is the last record at τ = 1 and still evaluates (#807).
//
// The fixture's three records cover ET 0–30. Before #815 every epoch in
// (30, 40) was answered — at ET 35, τ = 2, x 12 times its value at the end —
// because the record index that holds the final instant covers that whole
// next interval.
func TestChebyshevRecordRefusesEpochsPastItsInterval(t *testing.T) {
	for _, segType := range []int32{2, 3} {
		words, seg := chebyshevSegment(segType, 3)
		_, cached := readers(words)

		for _, r := range []*spk.Reader{newWordBuffer(words), cached} {
			if _, _, err := spk.EvaluateSegment(seg, r, 30); err != nil {
				t.Errorf("type %d: the final instant: %v", segType, err)
			}

			for _, et := range []float64{30.001, 35, 39.999} {
				if _, _, err := spk.EvaluateSegment(seg, r, et); !errors.Is(err, spk.ErrCorruptSPK) {
					t.Errorf("type %d at ET %v, past the final instant: err = %v, want ErrCorruptSPK", segType, et, err)
				}
			}
		}
	}
}

// TestChebyshevRecordRefusesAnEpochItsOwnIntervalDoesNotHold: the record the
// directory chooses must itself hold the epoch. A record whose stored middle
// disagrees with the directory is refused there rather than evaluated off its
// interval.
func TestChebyshevRecordRefusesAnEpochItsOwnIntervalDoesNotHold(t *testing.T) {
	words, seg := chebyshevSegment(2, 3)

	// Record 1 covers ET 10–20 by the directory; claim its middle is 35.
	rec := 2 + 3*4
	words[rec] = 35

	if _, _, err := spk.EvaluateSegment(seg, newWordBuffer(words), 12); !errors.Is(err, spk.ErrCorruptSPK) {
		t.Errorf("err = %v, want ErrCorruptSPK", err)
	}
}
