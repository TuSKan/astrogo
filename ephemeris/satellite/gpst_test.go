package satellite_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/satellite"
	"github.com/TuSKan/astrogo/time"
)

// TestGPSTimestampProducesTheSameStateAsItsUTC is the contract #145 asks for,
// and the one that makes the new scale worth its API surface.
//
// A GNSS receiver hands out GPS system time, which is 18 seconds ahead of UTC
// today. Before time.GPST existed a caller had no way to say so, and the only
// expressible option — call it UTC — put the satellite 18 seconds along its
// track. This asserts both halves: labelling it correctly gives the right
// state, and labelling it UTC gives a measurably wrong one.
//
// It lives here rather than in time/ because the arithmetic is only half the
// point. State converts to UTC internally, so this also proves the conversion
// survives the trip through timeToComponents rather than falling through a
// switch that never learned about the scale.
func TestGPSTimestampProducesTheSameStateAsItsUTC(t *testing.T) {
	t.Parallel()

	sat, err := satellite.NewFromTLE("ISS", valladoLine1, valladoLine2)
	if err != nil {
		t.Fatalf("NewFromTLE: %v", err)
	}

	// One physical instant, expressed two ways.
	utc := time.Date(2008, time.September, 20, 12, 0, 0, 0, time.LocationUTC)
	gps := utc.GPST()

	if gps.Scale() != time.GPST {
		t.Fatalf("precondition: converted instant is on scale %v", gps.Scale())
	}

	fromUTC, err := sat.State(0, utc)
	if err != nil {
		t.Fatalf("State(utc): %v", err)
	}

	fromGPS, err := sat.State(0, gps)
	if err != nil {
		t.Fatalf("State(gps): %v", err)
	}

	// Same instant, so the same state. The tolerance is a metre in AU, which
	// is far below the sub-second interpolation residual and far above float
	// noise.
	const metreInAU = 1.0 / 1.495978707e11

	if d := fromGPS.Pos.Sub(fromUTC.Pos).Norm(); d > metreInAU {
		t.Errorf("the same instant on two scales gave states %.3f km apart",
			d*1.495978707e8)
	}

	// Now the trap. Take the GPS label and assert it is UTC -- the only thing
	// a caller could do before this scale existed.
	misread := time.FromJD(gps.JD(), time.UTC)

	// The offset is NOT a constant, and this epoch shows why the scale has to
	// do the arithmetic rather than the caller. Vallado's element set is from
	// 2008, when delta-AT was 33 s, so GPST - UTC is 33 - 19 = 14 s here. At a
	// post-2017 epoch the same code gives 18. A caller subtracting a
	// remembered number would be four seconds -- 31 km -- wrong.
	gap := misread.Sub(utc).Seconds()
	if math.Abs(gap-14) > 1e-3 {
		t.Errorf("GPST - UTC = %.6f s at a 2008 epoch, want 14 (delta-AT 33 - 19)", gap)
	}

	fromMisread, err := sat.State(0, misread)
	if err != nil {
		t.Fatalf("State(misread): %v", err)
	}

	const kmPerAU = 1.495978707e8

	offKm := fromMisread.Pos.Sub(fromUTC.Pos).Norm() * kmPerAU
	speed := fromUTC.Vel.Norm() * kmPerAU / 86400

	// The displacement must be the chord the satellite actually covers in that
	// gap: slightly under gap*speed, since the orbit curves. Checking it
	// against the measured speed rather than a remembered figure is what makes
	// this an assertion instead of a transcription.
	arc := gap * speed
	if offKm > arc || offKm < 0.97*arc {
		t.Errorf("mislabelling GPS time as UTC moved the ISS %.1f km; expected just under "+
			"the %.1f km arc it covers in %.0f s at %.3f km/s", offKm, arc, gap, speed)
	}

	t.Logf("correctly labelled: %.6f km apart; mislabelled as UTC: %.1f km "+
		"(%.0f s at %.3f km/s)", fromGPS.Pos.Sub(fromUTC.Pos).Norm()*kmPerAU, offKm, gap, speed)

	// And the interval arithmetic that a scheduler would do on it: the two
	// labels differ by 18 s, while the instants do not differ at all.
	if d := math.Abs(gps.Sub(utc).Seconds()); d != 0 {
		t.Errorf("gps.Sub(utc) = %v s, want 0", d)
	}
}
