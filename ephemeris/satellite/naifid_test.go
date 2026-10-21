package satellite_test

import (
	"errors"
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/core"
	"github.com/TuSKan/astrogo/ephemeris/satellite"
	"github.com/TuSKan/astrogo/time"
)

// TestSatelliteAnswersToItsNAIFCode: a satellite is its NAIF code, −100000
// minus its NORAD number — the ISS is −125544 — and answers for that alone.
// It used to answer for 0, which is the Solar System barycenter's code now
// that an ID is a NAIF code (#809), so 0 is refused like any other body.
func TestSatelliteAnswersToItsNAIFCode(t *testing.T) {
	sat, err := satellite.NewFromTLE("ISS", issLine1, issLine2)
	if err != nil {
		t.Fatalf("NewFromTLE: %v", err)
	}

	if sat.ID() != -125544 {
		t.Fatalf("ID() = %d, want -125544", int32(sat.ID()))
	}

	at := time.Date(2026, 4, 19, 12, 0, 0, 0, time.LocationUTC)

	if _, err := sat.State(sat.ID(), at); err != nil {
		t.Errorf("State(its own code): %v", err)
	}

	for _, id := range []core.ID{core.SolarSystemBarycenter, core.Sun, 25544} {
		if _, err := sat.State(id, at); !errors.Is(err, satellite.ErrUnexpectedID) {
			t.Errorf("State(%v) = %v, want ErrUnexpectedID", id, err)
		}
	}
}
