//go:build integration

package jpl_test

import (
	"context"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/jpl"
	"github.com/TuSKan/astrogo/ephemeris/jpl/spk"
)

// TestEverySegmentHoldsItsLastInstant evaluates each of de440s's segments at
// its final epoch and one second before it. The state there must be the one a
// second earlier carried forward by its velocity: one second is far too short
// for the difference to be anything but motion, and a planetary record is a
// smooth polynomial.
//
// Each segment's final epoch used to be read one record past its end. On
// de440s that put the Earth 6×10⁶¹ km from the Earth-Moon barycenter at the
// last instant the kernel covers, with no error (#807).
func TestEverySegmentHoldsItsLastInstant(t *testing.T) {
	t.Parallel()

	p, err := jpl.NewProvider(context.Background(), core.Planets, "de440s")
	if err != nil {
		t.Fatalf("de440s: %v", err)
	}

	t.Cleanup(func() { _ = p.Close() })

	checked := 0

	for _, k := range p.Kernels {
		for i := range k.Segments {
			seg := &k.Segments[i]

			atEnd, vEnd, err := spk.EvaluateSegment(seg, k.Reader, seg.EndET)
			if err != nil {
				t.Errorf("target %d at its final epoch: %v", seg.Target, err)

				continue
			}

			before, vBefore, err := spk.EvaluateSegment(seg, k.Reader, seg.EndET-1)
			if err != nil {
				t.Fatalf("target %d a second before its final epoch: %v", seg.Target, err)
			}

			// A second of motion, carried at the mean of the two velocities:
			// exact for constant acceleration, and the Moon's departs from that
			// by micrometers over a second.
			predicted := before.Add(vBefore.Add(vEnd).MulScalar(0.5))

			if d := atEnd.Sub(predicted).Norm(); d > 1e-3 {
				t.Errorf("target %d (type %d): its final epoch is %.6g km from a second earlier carried forward, want under 1 m",
					seg.Target, seg.Type, d)
			}

			checked++
		}
	}

	if checked == 0 {
		t.Fatal("de440s had no segments to check")
	}
}
