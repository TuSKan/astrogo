package spk_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
)

// chebyshevWords is a type 2 or type 3 segment of len(values) records, each
// covering 10 s from ET 0 with a constant Chebyshev series (one coefficient)
// equal to values[i] on every axis, followed by the four-word directory with
// the given N.
func chebyshevWords(segType int32, values []float64, n float64) []float64 {
	perRecord := 3 // type 2: position x, y, z
	if segType == 3 {
		perRecord = 6 // type 3: position and velocity
	}

	var words []float64

	for i, v := range values {
		words = append(words, 10*float64(i)+5, 5) // mid, radius
		for range perRecord {
			words = append(words, v)
		}
	}

	return append(words, 0, 10, float64(2+perRecord), n) // INIT, INTLEN, RSIZE, N
}

// TestChebyshevSegmentHoldsItsFinalInstant: a segment's final epoch belongs to
// its last record. It used to index one past it and read the segment's own
// directory as coefficients, so de440s put the Earth 6×10⁶¹ km from its
// center at the last instant it covers, with no error (#807).
func TestChebyshevSegmentHoldsItsFinalInstant(t *testing.T) {
	t.Parallel()

	for _, segType := range []int32{2, 3} {
		words := chebyshevWords(segType, []float64{1, 2}, 2)
		r := newWordBuffer(words)
		seg := &spk.Segment{Type: segType, StartAddr: 1, EndAddr: int32(len(words)), StartET: 0, EndET: 20}

		for _, c := range []struct {
			et   float64
			want float64
		}{
			{0, 1},     // the first instant, in the first record
			{9.999, 1}, // just inside the first record
			{10, 2},    // a record boundary belongs to the record it opens
			{19.999, 2},
			{20, 2}, // the final instant, in the last record
		} {
			pos, _, err := spk.EvaluateSegment(seg, r, c.et)
			if err != nil {
				t.Errorf("type %d at ET %v: %v", segType, c.et, err)

				continue
			}

			if pos.X != c.want || pos.Y != c.want || pos.Z != c.want {
				t.Errorf("type %d at ET %v: %+v, want record value %v on every axis", segType, c.et, pos, c.want)
			}
		}
	}
}

// TestChebyshevSegmentRefusesADirectoryThatDoesNotDescribeTheEpoch: only the
// final instant is mapped back into the records. An epoch the directory puts
// outside them, or a record count the segment's words cannot hold, is
// corrupt data, never the nearest record's polynomial evaluated far outside
// the interval it fits.
func TestChebyshevSegmentRefusesADirectoryThatDoesNotDescribeTheEpoch(t *testing.T) {
	t.Parallel()

	for _, segType := range []int32{2, 3} {
		for _, c := range []struct {
			name string
			n    float64
			et   float64
			want error
		}{
			{"an epoch before INIT", 2, -1, spk.ErrCorruptSPK},
			{"an epoch past the final instant", 2, 31, spk.ErrCorruptSPK},
			{"an N the segment's words cannot hold", 3, 25, spk.ErrCorruptSPK},
			{"N of zero", 0, 5, spk.ErrInvalidRecordCount},
			{"a fractional N", 1.5, 5, spk.ErrInvalidRecordCount},
			{"a non-finite N", math.NaN(), 5, spk.ErrInvalidRecordCount},
		} {
			words := chebyshevWords(segType, []float64{1, 2}, c.n)
			r := newWordBuffer(words)
			seg := &spk.Segment{Type: segType, StartAddr: 1, EndAddr: int32(len(words)), StartET: -100, EndET: 100}

			if _, _, err := spk.EvaluateSegment(seg, r, c.et); !errors.Is(err, c.want) {
				t.Errorf("type %d, %s: err = %v, want %v", segType, c.name, err, c.want)
			}
		}
	}
}
