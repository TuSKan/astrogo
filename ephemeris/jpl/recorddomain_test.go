package jpl_test

import (
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
)

// TestEveryRecordBoundaryIsInsideItsRecord: the record-domain check (#815)
// refuses nothing a real kernel asks of it. For every type 2 and 3 segment of
// de440s, the epoch at every record boundary is evaluated on both records
// that meet there, and the segment's final instant on its last record, with
// no error. Segments with more than 3000 records are sampled evenly.
//
// Measured before the check existed, |τ| at these epochs never exceeds 1 in
// de440, de440s, de441, de442 or nep097: their stored middles and half-spans
// are exact. So this is the case the check's rounding allowance exists for,
// held to never being needed.
func TestEveryRecordBoundaryIsInsideItsRecord(t *testing.T) {
	t.Parallel()

	p := mustPlanetProvider(t)

	t.Cleanup(func() { _ = p.Close() })

	checked := 0

	for _, k := range p.Kernels {
		for i := range k.Segments {
			seg := &k.Segments[i]
			if seg.Type != 2 && seg.Type != 3 {
				continue
			}

			meta, err := k.Reader.ReadDoubles(seg.EndAddr-3, seg.EndAddr)
			if err != nil {
				t.Fatalf("target %d directory: %v", seg.Target, err)
			}

			init, intlen, n := meta[0], meta[1], int(meta[3])

			step := max(1, n/3000)

			evaluate := func(et float64) {
				if _, _, err := spk.EvaluateSegment(seg, k.Reader, et); err != nil {
					t.Fatalf("target %d at ET %v: %v", seg.Target, et, err)
				}

				checked++
			}

			for r := step; r < n; r += step {
				boundary := init + float64(r)*intlen

				// The boundary belongs to the record it opens; a hair before it,
				// to the record it closes, at that record's τ = 1.
				evaluate(boundary)
				evaluate(boundary - intlen*0x1p-40)
			}

			evaluate(seg.EndET)
		}
	}

	if checked == 0 {
		t.Fatal("de440s had no type 2 or 3 segment")
	}
}
