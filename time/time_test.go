package time_test

import (
	"math"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/iers"
	"github.com/TuSKan/astrogo/internal/testutil"
	atime "github.com/TuSKan/astrogo/time"
)

func TestFromJD(t *testing.T) {
	jd := 2460000.5
	tm := atime.FromJD(jd, atime.UTC)

	testutil.AssertNear(t, "JD value", tm.JD(), jd, 1e-15)
	testutil.AssertEqual(t, "Scale", tm.Scale(), atime.UTC)

	jd1, jd2 := tm.JDParts()
	if jd1+jd2 != jd {
		t.Errorf("JDParts sum %v != %v", jd1+jd2, jd)
	}
}

func TestFromJDParts(t *testing.T) {
	// 2460000.5 + 0.1
	tm := atime.FromJDParts(2460000.5, 0.1, atime.TAI)
	testutil.AssertNear(t, "Total JD", tm.JD(), 2460000.6, 1e-15)
	testutil.AssertEqual(t, "Scale", tm.Scale(), atime.TAI)

	// Normalization check: FromJDParts(2460000.5, 1.1)
	tm2 := atime.FromJDParts(2460000.5, 1.1, atime.UTC)
	j1, j2 := tm2.JDParts()
	// Total JD = 2460001.6. Normalization moves everything to jd2 except the integer part.
	testutil.AssertNear(t, "jd1 after norm", j1, 2460001.0, 1e-15)
	testutil.AssertNear(t, "jd2 after norm", j2, 0.6, 1e-15)
}

func TestFromGo(t *testing.T) {
	// 2000-01-01 12:00:00 UTC is exactly JD 2451545.0
	goTime := time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)
	tm := atime.FromGo(goTime)

	testutil.AssertNear(t, "JD for 2000-01-01 12:00", tm.JD(), 2451545.0, 1e-9)
	testutil.AssertEqual(t, "Scale", tm.Scale(), atime.UTC)
}

func TestNowUTC(t *testing.T) {
	tm := atime.NowUTC()
	if tm.JD() < 2460000 {
		t.Errorf("NowUTC JD seems too small: %v", tm.JD())
	}
	testutil.AssertEqual(t, "Scale", tm.Scale(), atime.UTC)
}

func TestArithmetic(t *testing.T) {
	tm := atime.FromJD(2450000.0, atime.TT)

	// AddDays
	tm2 := tm.AddDays(1.5)
	testutil.AssertNear(t, "Add 1.5 days", tm2.JD(), 2450001.5, 1e-15)
	testutil.AssertEqual(t, "Scale preserved", tm2.Scale(), atime.TT)

	// SubDays
	diff := tm2.SubDays(tm)
	testutil.AssertNear(t, "SubDays diff", diff, 1.5, 1e-15)
}

func TestScaleString(t *testing.T) {
	testutil.AssertEqual(t, "UTC string", atime.UTC.String(), "UTC")
	testutil.AssertEqual(t, "TAI string", atime.TAI.String(), "TAI")
}

func TestString(t *testing.T) {
	tm := atime.FromJD(2451545.0, atime.UTC)
	s := tm.String()
	if !math.IsNaN(tm.JD()) && (s == "" || s == "UNKNOWN") {
		t.Errorf("Time.String() returned %q", s)
	}
}

func TestScaleConversions(t *testing.T) {
	// J2000.0 UTC -> JD 2451545.0
	tm := atime.FromJD(2451545.0, atime.UTC)

	// In 2000, ΔAT = 32s
	// TT = UTC + 32s + 32.184s = UTC + 64.184s
	// TT_JD = 2451545.0 + 64.184 / 86400 = 2451545.0007428704

	tt := tm.TT()
	testutil.AssertEqual(t, "TT scale", tt.Scale(), atime.TT)
	testutil.AssertNear(t, "TT JD", tt.JD(), 2451545.0007428704, 1e-12)

	// TDB is approximated as TT in v1
	tdb := tm.TDB()
	testutil.AssertEqual(t, "TDB scale", tdb.Scale(), atime.TDB)
	testutil.AssertNear(t, "TDB JD", tdb.JD(), tt.JD(), 1e-15)
}

func TestTimeComparisons(t *testing.T) {
	t1 := atime.FromJD(2450000.0, atime.UTC)
	t2 := atime.FromJD(2450001.0, atime.UTC)
	t3 := atime.FromJD(2450000.0, atime.UTC)

	if !t1.Before(t2) {
		t.Errorf("Expected t1 before t2")
	}
	if !t2.After(t1) {
		t.Errorf("Expected t2 after t1")
	}
	if !t1.Equal(t3) {
		t.Errorf("Expected t1 equal to t3")
	}
	if t1.Equal(t2) {
		t.Errorf("Expected t1 not equal to t2")
	}

	zero := atime.Time{}
	if !zero.IsZero() {
		t.Errorf("Expected zero time to be zero")
	}
	if t1.IsZero() {
		t.Errorf("Expected t1 to not be zero")
	}
}

func TestTimeStdInterop(t *testing.T) {
	t1 := atime.FromJD(2451545.0, atime.UTC) // J2000
	gt := t1.ToGo()
	if gt.Year() != 2000 || gt.Month() != 1 || gt.Day() != 1 || gt.Hour() != 12 {
		t.Errorf("ToGo conversion failed, got %v", gt)
	}

	t2 := atime.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)
	if !t1.Equal(t2) {
		t.Errorf("Date constructor failed, expected %v got %v", t1.JD(), t2.JD())
	}

	fstr := t1.Format(time.RFC3339)
	if fstr != "2000-01-01T12:00:00Z" {
		t.Errorf("Format failed, got %q", fstr)
	}

	t3 := t1.Add(24 * time.Hour)
	testutil.AssertNear(t, "Add 24h", t3.JD(), 2451546.0, 1e-10)

	dur := t3.Sub(t1)
	if dur != 24*time.Hour {
		t.Errorf("Sub duration failed, expected 24h got %v", dur)
	}
}

func TestTimeFloatPrecisionRoundTrip(t *testing.T) {
	// Let's test a broad spectrum of "dirty" hours and dates
	// to ensure floating-point precision truncation never regresses again.

	datesToTest := []time.Time{
		time.Date(2026, 4, 6, 22, 0, 0, 0, time.UTC),                       // JD = 2461137.41666667 (The original issue)
		time.Date(2026, 4, 6, 19, 0, 0, 0, time.FixedZone("BRT", -3*3600)), // São Paulo Timezone directly
		time.Date(1999, 12, 31, 23, 59, 59, 0, time.UTC),                   // One second before Y2K
		time.Date(2038, 1, 19, 3, 14, 7, 0, time.UTC),                      // Year 2038 problem epoch
		time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC),                       // Exact .5 JD boundary
		time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),                        // Unix Epoch Origin
		time.Date(2050, 7, 4, 18, 30, 45, 123456000, time.UTC),             // Mixed sub-seconds
	}

	for _, dt := range datesToTest {
		// Go -> AstroGo Time
		astroTime := atime.FromGo(dt)

		// AstroGo Time -> Go
		roundTripped := astroTime.ToGo()

		// Verify exactly equal down to the second level at minimum
		if dt.Unix() != roundTripped.Unix() {
			t.Errorf("Round-trip failed for %v!\nExpected: %v\nGot:      %v",
				dt.Format(time.RFC3339Nano),
				dt.Format(time.RFC3339Nano),
				roundTripped.Format(time.RFC3339Nano))
		}
	}
}

type mockEOP struct{}

func (mockEOP) EOP(_ float64) (iers.EOP, error) {
	return iers.EOP{DUT1: 1.5}, nil
}

func TestTime_UT1(t *testing.T) {
	// Register the mock model
	iers.RegisterModel(mockEOP{})

	utc := atime.FromJD(2451545.0, atime.UTC) // J2000 UTC
	ut1 := utc.UT1()

	testutil.AssertEqual(t, "UT1 scale", ut1.Scale(), atime.UT1)

	expectedJD := 2451545.0 + (1.5 / 86400.0)
	testutil.AssertNear(t, "UT1 JD offset", ut1.JD(), expectedJD, 1e-12)

	// Calling UT1 on an existing UT1 struct should just return it unchanged
	ut1b := ut1.UT1()
	testutil.AssertEqual(t, "Idempotent UT1 scale", ut1b.Scale(), atime.UT1)
	testutil.AssertNear(t, "Idempotent UT1 JD", ut1b.JD(), ut1.JD(), 1e-15)
}

func TestTime_LocationPreservation(t *testing.T) {
	brt := time.FixedZone("BRT", -3*3600)

	// FromGo preserves the original location
	goTime := time.Date(2026, 4, 14, 22, 0, 0, 0, brt)
	tm := atime.FromGo(goTime)

	if tm.Location().String() != "BRT" {
		t.Errorf("expected BRT location, got %s", tm.Location())
	}

	// ToGo returns in the original timezone
	gt := tm.ToGo()
	if gt.Location().String() != "BRT" {
		t.Errorf("expected BRT in ToGo, got %s", gt.Location())
	}
	if gt.Hour() != 22 {
		t.Errorf("expected hour 22 in BRT, got %d", gt.Hour())
	}

	// Date preserves the location
	tm2 := atime.Date(2026, 4, 14, 22, 0, 0, 0, brt)
	if tm2.Location().String() != "BRT" {
		t.Errorf("expected BRT from Date, got %s", tm2.Location())
	}

	// Format uses the stored location
	fstr := tm2.Format("15:04")
	if fstr != "22:00" {
		t.Errorf("expected 22:00 in BRT format, got %s", fstr)
	}
}

func TestTime_In(t *testing.T) {
	tm := atime.FromJD(2451545.0, atime.UTC) // J2000 at UTC

	// Default location is UTC
	if tm.Location() != time.UTC {
		t.Errorf("expected UTC default, got %s", tm.Location())
	}

	// In() changes the display location without modifying the instant
	brt := time.FixedZone("BRT", -3*3600)
	tm2 := tm.In(brt)

	testutil.AssertNear(t, "JD unchanged by In()", tm2.JD(), tm.JD(), 1e-15)
	if tm2.Location().String() != "BRT" {
		t.Errorf("expected BRT after In(), got %s", tm2.Location())
	}

	// ToGo should now return in BRT
	gt := tm2.ToGo()
	if gt.Location().String() != "BRT" {
		t.Errorf("expected BRT in ToGo after In(), got %s", gt.Location())
	}
}

func TestTime_LocationPropagation(t *testing.T) {
	brt := time.FixedZone("BRT", -3*3600)
	tm := atime.Date(2026, 4, 14, 22, 0, 0, 0, brt)

	// AddDays preserves location
	tm2 := tm.AddDays(1)
	if tm2.Location().String() != "BRT" {
		t.Errorf("AddDays lost location: %s", tm2.Location())
	}

	// Add preserves location
	tm3 := tm.Add(24 * time.Hour)
	if tm3.Location().String() != "BRT" {
		t.Errorf("Add lost location: %s", tm3.Location())
	}

	// Scale conversions preserve location
	tt := tm.TT()
	if tt.Location().String() != "BRT" {
		t.Errorf("TT() lost location: %s", tt.Location())
	}

	tdb := tm.TDB()
	if tdb.Location().String() != "BRT" {
		t.Errorf("TDB() lost location: %s", tdb.Location())
	}

	iers.RegisterModel(mockEOP{})
	ut1 := tm.UT1()
	if ut1.Location().String() != "BRT" {
		t.Errorf("UT1() lost location: %s", ut1.Location())
	}
}
