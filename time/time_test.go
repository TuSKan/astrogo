package time_test

import (
	"math"
	"testing"
	"time"

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
