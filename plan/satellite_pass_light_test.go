package plan

import (
	"math"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/angle"
	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/epoch"
	"github.com/TuSKan/astrogo/unit"
)

// skyfieldSunlitAt is whether Skyfield's is_sunlit holds for the ISS element
// set at t, from skyfieldISSShadow's transitions, and how far t is from the
// nearest of them. The ISS is sunlit before the first, which it enters
// shadow at.
func skyfieldSunlitAt(t epoch.Time) (sunlit bool, margin unit.Duration) {
	sunlit, margin = true, unit.Days(1)

	for _, tr := range skyfieldISSShadow {
		at := epoch.Date(2026, time.April, tr.day, tr.hour, tr.minute, tr.second,
			int(tr.millisecond*1e6), time.UTC)

		margin = min(margin, t.Sub(at).Abs())

		if !t.Before(at) {
			sunlit = tr.sunlit
		}
	}

	return sunlit, margin
}

// TestSatellitePassEventsSayWhetherTheSatelliteIsSunlit: every rise,
// culmination and set of a day of ISS passes over Paranal says whether the
// ISS is sunlit, as Skyfield's is_sunlit says at that instant, and gives the
// Sun's altitude there (#1195). The day has events on both sides, sunlit and
// in shadow, or the check would cover only one. An event within 2 s of a
// shadow transition is left out, since astrogo's and Skyfield's transitions
// agree to about a second (TestSatelliteShadowAgreesWithSkyfield).
func TestSatellitePassEventsSayWhetherTheSatelliteIsSunlit(t *testing.T) {
	t.Parallel()

	sat, site := testISS(t)

	start := epoch.Date(2026, time.April, 19, 12, 0, 0, 0, time.UTC)

	passes, err := SatellitePasses(NewSatellite("ISS", sat.ID(), sat), siteAt(t, site), Window{Start: start, End: start.Add(unit.Days(1))}, WithMinElevation(angle.Deg(10)))
	if err != nil {
		t.Fatalf("SatellitePasses: %v", err)
	}

	var sunlit, shadowed int

	for _, p := range passes {
		for _, ev := range []struct {
			name string
			e    PassEvent
		}{{"rise", p.Rise}, {"culmination", p.Culmination}, {"set", p.Set}} {
			want, margin := skyfieldSunlitAt(ev.e.Time)
			if margin < unit.Seconds(2) {
				continue
			}

			if ev.e.Sunlit != want {
				t.Errorf("%s at %v: Sunlit = %v, Skyfield's is_sunlit %v", ev.name, ev.e.Time, ev.e.Sunlit, want)
			}

			if want {
				sunlit++
			} else {
				shadowed++
			}

			// The Sun's altitude, through a Context of its own and the Sun's
			// apparent place, which differs from the geometric one by the
			// aberration, about 20″.
			ctx := mustContext(t, ev.e.Time, site, defaultAtm)

			aa, err := observedPlace(NewSun(eph.Default()), ev.e.Time, ctx)
			if err != nil {
				t.Fatal(err)
			}

			if d := math.Abs(ev.e.SunAltitude.Degrees() - aa.Alt().Degrees()); d > 0.02 {
				t.Errorf("%s at %v: SunAltitude %.4f°, the Sun's place %.4f°", ev.name, ev.e.Time, ev.e.SunAltitude.Degrees(), aa.Alt().Degrees())
			}
		}
	}

	if sunlit == 0 || shadowed == 0 {
		t.Fatalf("%d sunlit and %d shadowed events over %d passes; the day no longer covers both", sunlit, shadowed, len(passes))
	}
}
