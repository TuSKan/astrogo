package time_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	"github.com/TuSKan/astrogo/time"
)

func TestTimeMJD(t *testing.T) {
	// J2000.0 = JD 2451545.0 = MJD 51544.5.
	tm := time.FromJD(2451545.0, time.UTC)
	testutil.AssertNear(t, "MJD at J2000.0", tm.MJD(), 51544.5, 1e-9)
}

func TestTimeGASTMatchesKnownValue(t *testing.T) {
	// Known GAST at J2000.0 ~280.46deg +-0.5deg (same reference value
	// plan.Site.LocalSiderealTime's own test validates against, since LST
	// at longitude 0 is exactly GAST).
	tm := time.FromJD(2451545.0, time.UTC)

	gast, err := tm.GAST()
	if err != nil {
		t.Fatalf("GAST failed: %v", err)
	}

	testutil.AssertNear(t, "GAST at J2000", gast.Degrees(), 280.46, 0.5)
}

func TestTimeJulianEpochYear(t *testing.T) {
	tm := time.FromJD(2451545.0, time.UTC)
	testutil.AssertNear(t, "JulianEpochYear at J2000.0", tm.JulianEpochYear(), 2000.0, 1e-9)
}

func TestTimeDayOfYear(t *testing.T) {
	jan1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.LocationUTC)
	testutil.AssertNear(t, "DayOfYear on Jan 1", jan1.DayOfYear(), 1.0, 1e-9)

	dec31 := time.Date(2025, 12, 31, 0, 0, 0, 0, time.LocationUTC)
	testutil.AssertNear(t, "DayOfYear on Dec 31 (non-leap year)", dec31.DayOfYear(), 365.0, 1e-9)
}

// TestDayOfYearIsInTheYearYearReads holds DayOfYear to the date Year, Month
// and Day describe, whatever the display location (#760).
//
// It measured from Jan 1 midnight in the display location while Year ignores
// it, so near the new year the two named different dates: 01:00 UTC on
// 1 January 2026 shown in São Paulo came out 0.917, below the first day, and
// 23:00 UTC on 31 December shown in Tokyo 366.33 in a 365-day year. Mid-year
// it was off by the zone's offset.
func TestDayOfYearIsInTheYearYearReads(t *testing.T) {
	t.Parallel()

	zones := []string{"UTC", "America/Sao_Paulo", "Asia/Tokyo", "Asia/Kolkata", "Pacific/Kiritimati", "Pacific/Pago_Pago"}

	instants := []time.GoTime{
		time.GoDate(2026, time.January, 1, 1, 0, 0, 0, time.LocationUTC),
		time.GoDate(2026, time.January, 1, 23, 30, 0, 0, time.LocationUTC),
		time.GoDate(2026, time.July, 1, 12, 0, 0, 0, time.LocationUTC),
		time.GoDate(2026, time.December, 31, 23, 0, 0, 0, time.LocationUTC),
		time.GoDate(2028, time.December, 31, 20, 0, 0, 0, time.LocationUTC),
	}

	for _, zone := range zones {
		loc, err := time.LoadLocation(zone)
		if err != nil {
			t.Fatalf("LoadLocation(%q): %v", zone, err)
		}

		for _, g := range instants {
			at := time.FromGo(g.In(loc))

			// The UTC calendar is the one Year reads for a UTC Time.
			want := float64(g.YearDay()) + float64(g.Hour()*3600+g.Minute()*60+g.Second())/86400

			if at.Year() != g.Year() {
				t.Fatalf("%s in %s: Year() = %d, want the UTC year %d", g.Format("2006-01-02 15:04"), zone, at.Year(), g.Year())
			}

			if got := at.DayOfYear(); math.Abs(got-want) > 1e-9 {
				t.Errorf("%s in %s: DayOfYear() = %.6f, want %.6f, the day of %d it falls on",
					g.Format("2006-01-02 15:04"), zone, got, want, at.Year())
			}
		}
	}
}

func TestTimeGASTFallsBackToUTCOnUT1Error(t *testing.T) {
	// The automatic lazy load's disk-read step must not find a real cache
	// file left by another test/run and silently overwrite errorEOP{}.
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	t.Cleanup(func() { remote.SetDataDir("") })

	time.RegisterModel(errorEOP{})

	defer time.ResetEOP()

	tm := time.FromJD(2451545.0, time.UTC)

	gast, err := tm.GAST()
	if err == nil {
		t.Fatal("expected an error when the registered model fails")
	}

	// Falling back to UTC in place of UT1 is at most a few hundred ms of
	// error — the result should still be close to the known GAST value.
	testutil.AssertNear(t, "GAST fallback at J2000", gast.Degrees(), 280.46, 0.5)
}

func TestTimeEOPDegradesToZeroWithWarning(t *testing.T) {
	// The automatic lazy load's disk-read step must not find a real cache
	// file left by another test/run and silently overwrite errorEOP{}.
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	t.Cleanup(func() { remote.SetDataDir("") })

	time.RegisterModel(errorEOP{})

	defer time.ResetEOP()

	tm := time.FromJD(2451545.0, time.UTC)

	eop := tm.EOP()
	if eop != (time.EOP{}) {
		t.Errorf("expected zero EOP on lookup failure, got %+v", eop)
	}
}

func TestTimeUTCFromUT1FallsBackOnEOPError(t *testing.T) {
	// The automatic lazy load's disk-read step must not find a real cache
	// file left by another test/run and silently overwrite errorEOP{}.
	remote.SetDataDir(testutil.FileURL(t, t.TempDir()))
	t.Cleanup(func() { remote.SetDataDir("") })

	time.RegisterModel(errorEOP{})

	defer time.ResetEOP()

	// A UT1-scale Time converting to UTC hits dut1OrFallback, which
	// substitutes DUT1=0 (UT1≈UTC) and logs a one-time warning on lookup
	// failure, rather than propagating the error like Time.UT1() does.
	ut1 := time.FromJD(2451545.0, time.UT1)

	utc := ut1.UTC()
	testutil.AssertNear(t, "UTC fallback (DUT1=0) at J2000", utc.JD(), 2451545.0, 1e-9)
}
