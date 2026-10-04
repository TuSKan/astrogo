package plan

import (
	"math"
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
)

// TestSeasonsIncludeNutation compares the 2027 equinoxes and solstices with
// Skyfield 1.54 (DE421, almanac.seasons), to the second. Without nutation in
// longitude (#414) they came out 4.6 to 5.4 minutes late; with it, and the
// Sun's apparent place from the ephemeris rather than a constant of
// aberration, they are within 0.4 s on the analytical ephemeris and with
// DE440s alike.
func TestSeasonsIncludeNutation(t *testing.T) {
	const tolSeconds = 2

	events, err := Seasons(2027, eph.Default())
	if err != nil {
		t.Fatal(err)
	}

	skyfield := []time.Time{
		time.Date(2027, 3, 20, 20, 24, 41, 0, time.LocationUTC),
		time.Date(2027, 6, 21, 14, 10, 50, 0, time.LocationUTC),
		time.Date(2027, 9, 23, 6, 1, 43, 0, time.LocationUTC),
		time.Date(2027, 12, 22, 2, 42, 10, 0, time.LocationUTC),
	}

	if len(events) != len(skyfield) {
		t.Fatalf("%d seasons in 2027, want 4", len(events))
	}

	for i, e := range events {
		if off := e.Time.Sub(skyfield[i]).Seconds(); math.Abs(off) > tolSeconds {
			t.Errorf("%v: %v, %+.1f s from Skyfield's", e.Season, e.Time, off)
		}
	}
}
