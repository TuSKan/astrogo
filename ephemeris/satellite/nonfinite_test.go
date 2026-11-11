package satellite_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/satellite"
	"github.com/TuSKan/astrogo/ephemeris/satellite/sgp4"
	"github.com/TuSKan/astrogo/time"
)

// TestLocateRefusesANonFiniteTime is #1052 at the level most callers reach it.
// A time whose Julian Date is NaN or infinite reached SGP4 as January 1st of
// the year 1, and a drag-free element set answered with a position there and no
// error.
func TestLocateRefusesANonFiniteTime(t *testing.T) {
	t.Parallel()

	const (
		line1 = "1 25544U 98067A   08264.51782528 -.00002182  00000-0 -11606-4 0  2927"
		line2 = "2 25544  51.6416 247.4627 0006703 130.5360 325.0288 15.72125391563537"
	)

	// B* zeroed and the check digit recomputed: without drag nothing else in
	// the model objected to the year 1.
	dragless := line1[:53] + " 00000-0" + line1[61:]
	dragless = dragless[:68] + string(rune('0'+sgp4.Checksum(dragless)))

	s, err := satellite.NewFromTLE("ISS without drag", dragless, line2)
	if err != nil {
		t.Fatal(err)
	}

	if b := s.Propagator().Elements().BStar; b != 0 {
		t.Fatalf("the splice left B* at %g; it no longer exercises the drag-free case", b)
	}

	for _, jd := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		st, err := s.Locate(s.ID(), time.FromJD(jd, time.UTC))
		if !errors.Is(err, sgp4.ErrNotFinite) {
			t.Errorf("Locate at JD %g returned %v, want one wrapping sgp4.ErrNotFinite", jd, err)
		}

		if st != (core.State{}) {
			t.Errorf("Locate at JD %g returned the state %+v alongside its error", jd, st)
		}
	}
}
