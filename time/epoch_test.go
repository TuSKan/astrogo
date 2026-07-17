package time_test

import (
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
	atime "github.com/TuSKan/astrogo/time"
)

func TestTimeMJD(t *testing.T) {
	// J2000.0 = JD 2451545.0 = MJD 51544.5.
	tm := atime.FromJD(2451545.0, atime.UTC)
	testutil.AssertNear(t, "MJD at J2000.0", tm.MJD(), 51544.5, 1e-9)
}

func TestTimeGASTMatchesKnownValue(t *testing.T) {
	// Known GAST at J2000.0 ~280.46deg +-0.5deg (same reference value
	// plan.Site.LocalSiderealTime's own test validates against, since LST
	// at longitude 0 is exactly GAST).
	tm := atime.FromJD(2451545.0, atime.UTC)

	gast, err := tm.GAST()
	if err != nil {
		t.Fatalf("GAST failed: %v", err)
	}

	testutil.AssertNear(t, "GAST at J2000", gast.Degrees(), 280.46, 0.5)
}

func TestTimeJulianEpochYear(t *testing.T) {
	tm := atime.FromJD(2451545.0, atime.UTC)
	testutil.AssertNear(t, "JulianEpochYear at J2000.0", tm.JulianEpochYear(), 2000.0, 1e-9)
}

func TestTimeDayOfYear(t *testing.T) {
	jan1 := atime.Date(2025, 1, 1, 0, 0, 0, 0, atime.LocationUTC)
	testutil.AssertNear(t, "DayOfYear on Jan 1", jan1.DayOfYear(), 1.0, 1e-9)

	dec31 := atime.Date(2025, 12, 31, 0, 0, 0, 0, atime.LocationUTC)
	testutil.AssertNear(t, "DayOfYear on Dec 31 (non-leap year)", dec31.DayOfYear(), 365.0, 1e-9)
}

func TestTimeGASTFallsBackToUTCOnUT1Error(t *testing.T) {
	atime.RegisterModel(errorEOP{})
	defer atime.RegisterModel(atime.ZeroModel{})

	tm := atime.FromJD(2451545.0, atime.UTC)

	gast, err := tm.GAST()
	if err == nil {
		t.Fatal("expected an error when the registered model fails")
	}

	// Falling back to UTC in place of UT1 is at most a few hundred ms of
	// error — the result should still be close to the known GAST value.
	testutil.AssertNear(t, "GAST fallback at J2000", gast.Degrees(), 280.46, 0.5)
}

func TestTimeEOPDegradesToZeroWithWarning(t *testing.T) {
	atime.RegisterModel(errorEOP{})
	defer atime.RegisterModel(atime.ZeroModel{})

	tm := atime.FromJD(2451545.0, atime.UTC)

	eop := tm.EOP()
	if eop != (atime.EOP{}) {
		t.Errorf("expected zero EOP on lookup failure, got %+v", eop)
	}
}

func TestTimeUTCFromUT1FallsBackOnEOPError(t *testing.T) {
	atime.RegisterModel(errorEOP{})
	defer atime.RegisterModel(atime.ZeroModel{})

	// A UT1-scale Time converting to UTC hits dut1OrFallback, which
	// substitutes DUT1=0 (UT1≈UTC) and logs a one-time warning on lookup
	// failure, rather than propagating the error like Time.UT1() does.
	ut1 := atime.FromJD(2451545.0, atime.UT1)

	utc := ut1.UTC()
	testutil.AssertNear(t, "UTC fallback (DUT1=0) at J2000", utc.JD(), 2451545.0, 1e-9)
}
