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

	passes, err := SatellitePasses(sat, sat.ID(), "ISS", start, start.Add(unit.Days(2)), observer, angle.Deg(10))
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

		// The window opens and closes with the ISS below 10°, so every one
		// of these is a whole pass.
		if p.RiseCutOff || p.SetCutOff {
			t.Errorf("pass %d is marked cut off (rise %v, set %v) inside a window that holds all of it",
				i+1, p.RiseCutOff, p.SetCutOff)
		}
	}
}

// TestSatellitePassesCutOffAtTheWindowEdges: a pass in progress when the
// window opens or closes is reported, cut off at that edge and marked so.
// Until #896 it was dropped, and a window opened during the 79.6° pass found
// nothing.
//
// The pass is Skyfield's third above, 04-20 from 03:03:34.10 to 03:10:17.07,
// culminating at 03:06:56.58. Its real ends are held to Skyfield's to the
// second, as TestSatellitePassesAgreeWithSkyfield holds them, and its cut
// ends are the window's own edges exactly.
func TestSatellitePassesCutOffAtTheWindowEdges(t *testing.T) {
	t.Parallel()

	sat := newISSProvider(t)

	observer, err := coord.NewGeodetic(angle.Deg(-46.63), angle.Deg(-23.55), unit.Meters(760))
	if err != nil {
		t.Fatal(err)
	}

	at := func(h, m int, s float64) time.Time {
		whole := math.Floor(s)

		return time.Date(2026, time.April, 20, h, m, int(whole), int((s-whole)*1e9), time.LocationUTC)
	}

	rise, culmination, set := at(3, 3, 34.10), at(3, 6, 56.58), at(3, 10, 17.07)

	for _, tc := range []struct {
		name                        string
		start, end                  time.Time
		riseCutOff, setCutOff       bool
		wantRise, wantCulm, wantSet time.Time
		culmIsPeak                  bool // whether the window holds the real culmination
	}{
		{"opens mid-pass", at(3, 6, 0), at(4, 0, 0), true, false, at(3, 6, 0), culmination, set, true},
		{"closes mid-pass", at(2, 0, 0), at(3, 6, 0), false, true, rise, at(3, 6, 0), at(3, 6, 0), false},
		{"lies inside the pass", at(3, 5, 0), at(3, 8, 0), true, true, at(3, 5, 0), culmination, at(3, 8, 0), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			passes, err := SatellitePasses(sat, sat.ID(), "ISS", tc.start, tc.end, observer, angle.Deg(10))
			if err != nil {
				t.Fatalf("SatellitePasses: %v", err)
			}

			if len(passes) != 1 {
				t.Fatalf("%d passes, want the one the window cuts", len(passes))
			}

			p := passes[0]

			if p.RiseCutOff != tc.riseCutOff || p.SetCutOff != tc.setCutOff {
				t.Errorf("RiseCutOff %v, SetCutOff %v; want %v, %v", p.RiseCutOff, p.SetCutOff, tc.riseCutOff, tc.setCutOff)
			}

			// A cut end is the window's edge exactly; a real one is
			// Skyfield's crossing, to the second.
			for _, e := range []struct {
				name      string
				got, want time.Time
				cut       bool
			}{
				{"rise", p.Rise.Time, tc.wantRise, tc.riseCutOff},
				{"set", p.Set.Time, tc.wantSet, tc.setCutOff},
			} {
				off := e.got.Sub(e.want).Seconds()

				if e.cut && off != 0 {
					t.Errorf("cut %s at %v, want the window's edge %v", e.name, e.got, e.want)
				}

				if !e.cut && math.Abs(off) > 1 {
					t.Errorf("%s %+.2f s from Skyfield's", e.name, off)
				}
			}

			if off := p.Culmination.Time.Sub(tc.wantCulm).Seconds(); math.Abs(off) > 1 {
				t.Errorf("culmination %+.2f s from %v", off, tc.wantCulm)
			}

			// Inside the window the highest point is the real one; cut before
			// it, the highest point is the window's end, lower than 79.625°.
			if got := p.Culmination.Elevation.Degrees(); tc.culmIsPeak && math.Abs(got-79.625) > 0.01 {
				t.Errorf("highest elevation %.3f°, Skyfield 79.625°", got)
			} else if !tc.culmIsPeak && got >= 79.625 {
				t.Errorf("highest elevation %.3f° in a window that ends before the 79.625° culmination", got)
			}

			if want, _ := time.ToGoDuration(p.Set.Time.Sub(p.Rise.Time)); p.Duration != want {
				t.Errorf("Duration %v, want the %v from Rise to Set", p.Duration, want)
			}
		})
	}
}

// TestSatellitePassesFindAPassThatBarelyClearsTheThreshold: a pass above
// minElevation for less than the 30 s sampling step can fall between two
// samples below it, and until #897 it was found or not depending on where the
// window started.
//
// Skyfield's fourth pass above culminates at 13.852° at 16:30:05.01. Above
// 13.84° it is up for about a dozen seconds, so thirty window starts a second
// apart put the samples everywhere across it, and each must find it. Above
// 13.86°, just over the peak, none may.
func TestSatellitePassesFindAPassThatBarelyClearsTheThreshold(t *testing.T) {
	t.Parallel()

	sat := newISSProvider(t)

	observer, err := coord.NewGeodetic(angle.Deg(-46.63), angle.Deg(-23.55), unit.Meters(760))
	if err != nil {
		t.Fatal(err)
	}

	culmination := time.Date(2026, time.April, 20, 16, 30, 5, 10_000_000, time.LocationUTC)

	for offset := range 30 {
		start := time.Date(2026, time.April, 20, 16, 0, offset, 0, time.LocationUTC)
		end := start.Add(unit.Seconds(3600))

		passes, err := SatellitePasses(sat, sat.ID(), "ISS", start, end, observer, angle.Deg(13.84))
		if err != nil {
			t.Fatalf("SatellitePasses from 16:00:%02d: %v", offset, err)
		}

		if len(passes) != 1 {
			t.Errorf("window from 16:00:%02d: %d passes above 13.84°, want the one culminating at 13.852°",
				offset, len(passes))

			continue
		}

		p := passes[0]

		if !p.Rise.Time.Before(p.Culmination.Time) || !p.Culmination.Time.Before(p.Set.Time) {
			t.Errorf("window from 16:00:%02d: rise %v, culmination %v and set %v are out of order",
				offset, p.Rise.Time, p.Culmination.Time, p.Set.Time)
		}

		if off := p.Culmination.Time.Sub(culmination).Seconds(); math.Abs(off) > 1 {
			t.Errorf("window from 16:00:%02d: culmination %+.2f s from Skyfield's", offset, off)
		}

		if d := math.Abs(p.Culmination.Elevation.Degrees() - 13.852); d > 0.01 {
			t.Errorf("window from 16:00:%02d: highest elevation %.3f°, Skyfield 13.852°",
				offset, p.Culmination.Elevation.Degrees())
		}

		above, err := SatellitePasses(sat, sat.ID(), "ISS", start, end, observer, angle.Deg(13.86))
		if err != nil {
			t.Fatalf("SatellitePasses from 16:00:%02d above 13.86°: %v", offset, err)
		}

		if len(above) != 0 {
			t.Errorf("window from 16:00:%02d: %d passes above 13.86°, over a 13.852° peak", offset, len(above))
		}
	}
}
