package plan

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

// TestSatellitePassesAgreeWithSkyfield holds SatellitePasses to Skyfield
// 1.55's EarthSatellite.find_events on the same ISS element set
// (issLine1/issLine2), from São Paulo (−23.55°, −46.63°, 760 m) above 10°,
// over the two days after the element set's epoch. The instants and
// elevations below are Skyfield's, computed on 2026-10-06.
//
// SGP4 is the same model on both sides and is reproduced against Vallado's
// suite in ephemeris/satellite/sgp4, so what this checks is the pass finder
// on top of it: the sampling, the root-finding at rise and set, and
// findCulmination. Both sides use geometric altitude — SatellitePasses
// applies no refraction, and nor does Skyfield's altaz.
//
// Measured, rise and set are within 0.38 s and the highest elevation within
// 0.005°; the culmination instant within 0.49 s, which is findCulmination's
// one-second refinement grid (#558). The azimuth at culmination is not
// compared: on the 79.6° pass half a second near the zenith is 2.7° of
// azimuth, which is geometry, not disagreement.
func TestSatellitePassesAgreeWithSkyfield(t *testing.T) {
	t.Parallel()

	sat := newISSProvider(t)

	observer, err := coord.NewGeodetic(angle.Deg(-46.63), angle.Deg(-23.55), unit.Meters(760))
	if err != nil {
		t.Fatal(err)
	}

	start := time.Date(2026, time.April, 19, 12, 0, 0, 0, time.LocationUTC)

	passes, err := SatellitePasses(sat, "ISS", start, start.Add(unit.Days(2)), observer, angle.Deg(10))
	if err != nil {
		t.Fatalf("SatellitePasses: %v", err)
	}

	at := func(d, h, m int, s float64) time.Time {
		whole := math.Floor(s)

		return time.Date(2026, time.April, d, h, m, int(whole), int((s-whole)*1e9), time.LocationUTC)
	}

	want := []struct {
		rise, culm, set time.Time
		maxElevDeg      float64
	}{
		{at(19, 17, 14, 18.99), at(19, 17, 17, 15.19), at(19, 17, 20, 12.74), 27.463},
		{at(19, 18, 51, 54.58), at(19, 18, 54, 3.31), at(19, 18, 56, 12.76), 15.614},
		{at(20, 3, 3, 34.10), at(20, 3, 6, 56.58), at(20, 3, 10, 17.07), 79.625},
		{at(20, 16, 28, 15.44), at(20, 16, 30, 5.01), at(20, 16, 31, 55.16), 13.852},
		{at(20, 18, 3, 40.55), at(20, 18, 6, 39.53), at(20, 18, 9, 39.93), 27.982},
		{at(21, 2, 16, 23.43), at(21, 2, 19, 39.29), at(21, 2, 22, 53.29), 45.441},
	}

	if len(passes) != len(want) {
		t.Fatalf("%d passes, Skyfield finds %d", len(passes), len(want))
	}

	// One second for every instant: rise and set are refined well inside it,
	// and the culmination is chosen on a one-second grid.
	const tolSeconds = 1.0

	for i, w := range want {
		p := passes[i]

		for _, e := range []struct {
			name      string
			got, want time.Time
		}{
			{"rise", p.Rise.Time, w.rise},
			{"culmination", p.Culmination.Time, w.culm},
			{"set", p.Set.Time, w.set},
		} {
			if off := e.got.Sub(e.want).Seconds(); math.Abs(off) > tolSeconds {
				t.Errorf("pass %d %s: %+.2f s from Skyfield's", i+1, e.name, off)
			}
		}

		if d := math.Abs(p.Culmination.Elevation.Degrees() - w.maxElevDeg); d > 0.01 {
			t.Errorf("pass %d: highest elevation %.3f°, Skyfield %.3f°", i+1, p.Culmination.Elevation.Degrees(), w.maxElevDeg)
		}
	}
}
