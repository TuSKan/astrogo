package time_test

import (
	"math"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/earth"
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

type mockEOP struct{}

func (mockEOP) EOP(_ float64) (earth.EOP, error) {
	return earth.EOP{DUT1: 1.5}, nil
}

func TestTime_UT1(t *testing.T) {
	// Register the mock model
	earth.RegisterModel(mockEOP{})
	
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
