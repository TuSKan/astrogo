package time_test

import (
	"math"
	"testing"
	"time"

	"github.com/TuSKan/astrogo/internal/testutil"
	"github.com/TuSKan/astrogo/remote"
	atime "github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/time/internal/iers"
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

	// TDB uses Fairhead & Bretagnon correction.
	// At J2000 (T=0): g = 357.5277233°, TDB−TT ≈ −71.5 μs
	tdb := tm.TDB()
	testutil.AssertEqual(t, "TDB scale", tdb.Scale(), atime.TDB)

	// Compute TDB−TT correction in seconds via two-part JD differencing.
	tt1, tt2 := tt.JDParts()
	tdb1, tdb2 := tdb.JDParts()
	tdbMinusTTsec := ((tdb1 - tt1) + (tdb2 - tt2)) * 86400.0

	// The XJSE2000 multi-term correction at J2000 (T=0) should be small (~89 µs).
	// The dominant term has amplitude 1.657 ms, so |TDB−TT| must be < 2 ms.
	if math.Abs(tdbMinusTTsec) > 0.002 {
		t.Errorf("TDB−TT = %e s exceeds 2 ms bound", tdbMinusTTsec)
	}

	// TDB must differ from TT (not just relabeled)
	if math.Abs(tdbMinusTTsec) < 1e-8 {
		t.Errorf("TDB and TT should differ by XJSE2000 correction, but Δ = %e s", tdbMinusTTsec)
	}
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
	// The automatic lazy load's disk-read step must not find a real cache
	// file left by another test/run and silently overwrite mockEOP{}.
	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	// Register the mock model
	iers.RegisterModel(mockEOP{})
	t.Cleanup(func() { iers.RegisterModel(iers.ZeroModel{}) })

	utc := atime.FromJD(2451545.0, atime.UTC) // J2000 UTC

	ut1, err := utc.UT1()
	if err != nil {
		t.Fatalf("UT1() returned error: %v", err)
	}

	testutil.AssertEqual(t, "UT1 scale", ut1.Scale(), atime.UT1)

	expectedJD := 2451545.0 + (1.5 / 86400.0)
	testutil.AssertNear(t, "UT1 JD offset", ut1.JD(), expectedJD, 1e-12)

	// Calling UT1 on an existing UT1 struct should just return it unchanged
	ut1b, err := ut1.UT1()
	if err != nil {
		t.Fatalf("UT1() on UT1 time returned error: %v", err)
	}

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
	// The automatic lazy load's disk-read step must not find a real cache
	// file left by another test/run and silently swap out mockEOP{}.
	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })
	t.Cleanup(func() { iers.RegisterModel(iers.ZeroModel{}) })

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

	ut1, err := tm.UT1()
	if err != nil {
		t.Fatal(err)
	}

	if ut1.Location().String() != "BRT" {
		t.Errorf("UT1() lost location: %s", ut1.Location())
	}

	// TAI and UTC conversions also preserve location
	tai := tm.TAI()
	if tai.Location().String() != "BRT" {
		t.Errorf("TAI() lost location: %s", tai.Location())
	}

	utcBack := tt.UTC()
	if utcBack.Location().String() != "BRT" {
		t.Errorf("TT().UTC() lost location: %s", utcBack.Location())
	}
}

// ── New Scale Conversion Tests ───────────────────────────────────────────────

func TestTAI_Conversion(t *testing.T) {
	// J2000.0 UTC: ΔAT = 32 leap seconds
	utc := atime.FromJD(2451545.0, atime.UTC)

	tai := utc.TAI()
	testutil.AssertEqual(t, "TAI scale", tai.Scale(), atime.TAI)
	// TAI = UTC + 32s
	expectedTAI := 2451545.0 + 32.0/86400.0
	testutil.AssertNear(t, "TAI JD", tai.JD(), expectedTAI, 1e-12)

	// TAI → TT should add 32.184s
	tt := tai.TT()
	testutil.AssertEqual(t, "TT from TAI scale", tt.Scale(), atime.TT)

	expectedTT := expectedTAI + 32.184/86400.0
	testutil.AssertNear(t, "TT from TAI JD", tt.JD(), expectedTT, 1e-12)

	// Idempotent
	tai2 := tai.TAI()
	testutil.AssertNear(t, "TAI idempotent", tai2.JD(), tai.JD(), 1e-15)
}

func TestUTC_Inverse(t *testing.T) {
	utc := atime.FromJD(2451545.0, atime.UTC)

	// UTC → TT → UTC round-trip
	tt := utc.TT()
	utcBack := tt.UTC()
	testutil.AssertEqual(t, "UTC round-trip scale", utcBack.Scale(), atime.UTC)
	testutil.AssertNear(t, "UTC→TT→UTC", utcBack.JD(), utc.JD(), 1e-12)

	// UTC → TAI → UTC round-trip
	tai := utc.TAI()
	utcBack2 := tai.UTC()
	testutil.AssertEqual(t, "UTC→TAI→UTC scale", utcBack2.Scale(), atime.UTC)
	testutil.AssertNear(t, "UTC→TAI→UTC", utcBack2.JD(), utc.JD(), 1e-12)

	// UTC → TDB → UTC round-trip
	tdb := utc.TDB()
	utcBack3 := tdb.UTC()
	testutil.AssertEqual(t, "UTC→TDB→UTC scale", utcBack3.Scale(), atime.UTC)
	testutil.AssertNear(t, "UTC→TDB→UTC", utcBack3.JD(), utc.JD(), 1e-10)

	// Idempotent
	utc2 := utc.UTC()
	testutil.AssertNear(t, "UTC idempotent", utc2.JD(), utc.JD(), 1e-15)
}

func TestConversionRoundTrips(t *testing.T) {
	// The automatic lazy load's disk-read step must not find a real cache
	// file left by another test/run and silently swap out mockEOP{}.
	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })
	t.Cleanup(func() { iers.RegisterModel(iers.ZeroModel{}) })

	// Full chain: UTC → TAI → TT → TDB → TT → TAI → UTC
	utc := atime.FromJD(2460000.5, atime.UTC)

	tai := utc.TAI()
	tt := tai.TT()
	tdb := tt.TDB()
	ttBack := tdb.TT()
	taiBack := ttBack.TAI()
	utcBack := taiBack.UTC()

	testutil.AssertEqual(t, "Full round-trip scale", utcBack.Scale(), atime.UTC)
	// Allow 1e-10 JD tolerance for TDB→TT round-trip (FB is not exactly invertible)
	testutil.AssertNear(t, "Full round-trip JD", utcBack.JD(), utc.JD(), 1e-10)

	// With UT1
	iers.RegisterModel(mockEOP{})

	ut1, err := utc.UT1()
	if err != nil {
		t.Fatal(err)
	}

	utcFromUT1 := ut1.UTC()
	testutil.AssertEqual(t, "UT1→UTC scale", utcFromUT1.Scale(), atime.UTC)
	// DUT1=1.5s: UTC→UT1→UTC should recover within ~1e-12 JD
	testutil.AssertNear(t, "UTC→UT1→UTC", utcFromUT1.JD(), utc.JD(), 1e-12)
}

func TestTDB_XJSE2000(t *testing.T) {
	// Test at several epochs to verify the multi-term XJSE2000 correction.
	// We compare using JDParts() instead of JD() to avoid float64
	// cancellation (the correction ~1.7ms is near the noise floor of
	// the combined JD's 16 significant digits).
	epochs := []struct {
		name string
		jd   float64
	}{
		{"J2000", 2451545.0},
		{"2010-01-01", 2455197.5},
		{"2020-07-01", 2459031.5},
		{"2026-04-06", 2461136.5},
		{"1990-01-01", 2447892.5},
	}

	for _, ep := range epochs {
		t.Run(ep.name, func(t *testing.T) {
			utc := atime.FromJD(ep.jd, atime.UTC)
			tt := utc.TT()
			tdb := utc.TDB()

			// Use two-part JD for precise differencing
			tdb1, tdb2 := tdb.JDParts()
			tt1, tt2 := tt.JDParts()
			tdbMinusTTsec := ((tdb1 - tt1) + (tdb2 - tt2)) * 86400.0

			// XJSE2000 amplitude is dominated by 1.657 ms; the correction must be within bounds.
			if math.Abs(tdbMinusTTsec) > 0.002 {
				t.Errorf("TDB−TT = %.6f s exceeds 2 ms bound", tdbMinusTTsec)
			}

			// Verify the dominant term is within 30 µs of single-term approximation
			// (the multi-term series adds corrections up to ~22 µs from other planets).
			T := ((tt1 - 2451545.0) + tt2) / 36525.0
			g := (357.5277233 + 35999.0503400*T) * math.Pi / 180.0
			singleTermSec := 0.001657 * math.Sin(g)

			diff := math.Abs(tdbMinusTTsec - singleTermSec)
			if diff > 50e-6 {
				t.Errorf("Multi-term deviates from single-term by %.1f µs (expected < 50 µs)", diff*1e6)
			}

			t.Logf("T=%.6f  TDB−TT = %.6f ms  (single-term: %.6f ms, Δ=%.2f µs)",
				T, tdbMinusTTsec*1000, singleTermSec*1000, diff*1e6)
		})
	}
}

func TestCrossScaleComparison(t *testing.T) {
	// Create the same instant in different scales
	utc := atime.FromJD(2451545.0, atime.UTC)
	tt := utc.TT()
	tai := utc.TAI()
	tdb := utc.TDB()

	// Same instant, different scales: Before/After should be false
	if utc.Before(tt) {
		t.Error("UTC should not be Before TT (same instant)")
	}

	if utc.After(tt) {
		t.Error("UTC should not be After TT (same instant)")
	}

	if !utc.Equal(tt) {
		t.Error("UTC and TT representing same instant should be Equal")
	}

	// Cross-scale Sub should be ~0
	delta := utc.Sub(tt)
	if math.Abs(delta.Seconds()) > 1e-6 {
		t.Errorf("UTC.Sub(TT) for same instant: got %v, expected ~0", delta)
	}

	// TAI vs TDB (same instant — may have ~1ns FB round-trip residual)
	taiTdbDelta := tai.Sub(tdb)
	if math.Abs(taiTdbDelta.Seconds()) > 1e-6 {
		t.Errorf("TAI.Sub(TDB) for same instant: got %v, expected ~0", taiTdbDelta)
	}

	// Different instants across scales
	utc2 := utc.AddDays(1) // 1 day later, still UTC
	if !utc.Before(utc2.TT()) {
		t.Error("Earlier UTC should be Before later TT")
	}

	if !utc2.TDB().After(utc) {
		t.Error("Later TDB should be After earlier UTC")
	}

	// SubDays cross-scale
	daysDiff := utc2.TDB().SubDays(utc)
	testutil.AssertNear(t, "Cross-scale SubDays", daysDiff, 1.0, 1e-8)
}

func TestCrossScaleSub(t *testing.T) {
	// Verify Sub handles same-scale fast path correctly
	a := atime.FromJD(2451545.0, atime.TT)
	b := atime.FromJD(2451546.0, atime.TT)

	dur := b.Sub(a)
	if dur != 24*time.Hour {
		t.Errorf("Same-scale Sub: got %v, want 24h", dur)
	}

	// Cross-scale Sub
	utc := atime.FromJD(2451545.0, atime.UTC)
	tt := utc.TT()

	dur2 := utc.Sub(tt)
	if math.Abs(dur2.Seconds()) > 1e-6 {
		t.Errorf("Cross-scale Sub for same instant: got %v", dur2)
	}
}

type errorEOP struct{}

func (errorEOP) EOP(_ float64) (iers.EOP, error) {
	return iers.EOP{}, &eopUnavailableError{}
}

type eopUnavailableError struct{}

func (e *eopUnavailableError) Error() string { return "EOP data unavailable" }

func TestUT1_Error(t *testing.T) {
	// The automatic lazy load's disk-read step must not find a real cache
	// file left by another test/run and silently overwrite errorEOP{}.
	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	// Register a model that always fails
	iers.RegisterModel(errorEOP{})
	defer iers.RegisterModel(iers.ZeroModel{}) // restore

	utc := atime.FromJD(2451545.0, atime.UTC)

	_, err := utc.UT1()
	if err == nil {
		t.Fatal("Expected error from UT1() with unavailable EOP, got nil")
	}

	t.Logf("UT1 error (expected): %v", err)
}

func TestTT_FromAllScales(t *testing.T) {
	// The automatic lazy load's disk-read step must not find a real cache
	// file left by another test/run and silently swap out mockEOP{}.
	remote.SetDataDirPath(t.TempDir())
	t.Cleanup(func() { remote.SetDataDir("") })

	iers.RegisterModel(mockEOP{})
	defer iers.RegisterModel(iers.ZeroModel{})

	utc := atime.FromJD(2451545.0, atime.UTC)
	expectedTT := utc.TT()

	// TAI → TT
	tai := utc.TAI()
	ttFromTAI := tai.TT()
	testutil.AssertNear(t, "TAI→TT", ttFromTAI.JD(), expectedTT.JD(), 1e-14)

	// TDB → TT
	tdb := utc.TDB()
	ttFromTDB := tdb.TT()
	// TDB→TT→TDB→TT may have ~1e-10 JD residual from FB non-invertibility
	testutil.AssertNear(t, "TDB→TT", ttFromTDB.JD(), expectedTT.JD(), 1e-10)

	// UT1 → TT (goes UT1→UTC→TT)
	ut1, err := utc.UT1()
	if err != nil {
		t.Fatal(err)
	}

	ttFromUT1 := ut1.TT()
	testutil.AssertNear(t, "UT1→TT", ttFromUT1.JD(), expectedTT.JD(), 1e-12)
}

// TestTT_Pre1972UsesDeltaTNotLeapSeconds is a regression test: 1960-1971 dates
// were previously misclassified as "modern" (leap-second era) because SOFA's
// Dat returns nonzero rational drift-rate values from 1960 onward, not 0 until
// 1972 as the old gate assumed, so this window silently used ΔAT+32.184s (the
// drift-rate table) instead of the intended ΔT polynomial. Note: across this
// whole window the two formulas happen to agree to within ~0.01-0.13s (both
// track the same era's Earth-rotation/atomic-time divergence), so the
// practical numerical impact is small — this test exists to confirm the
// *formula* matches the code's documented design intent, not to demonstrate a
// large numerical discrepancy.
func TestTT_Pre1972UsesDeltaTNotLeapSeconds(t *testing.T) {
	utc := atime.FromGo(time.Date(1965, 6, 15, 0, 0, 0, 0, time.UTC))
	tt := utc.TT()

	offsetSeconds := (tt.JD() - utc.JD()) * 86400.0
	expectedDT := atime.DeltaT(utc.DecimalYear())

	// 1e-4s tolerance absorbs float64 noise from JD() collapsing the
	// two-part representation, while still being tighter than the ~0.09s
	// gap between ΔT and the old (buggy) drift-table value at this date.
	testutil.AssertNear(t, "TT-UTC offset (ΔT-based)", offsetSeconds, expectedDT, 1e-4)

	// The true 1972-01-01 boundary itself must still use the leap-second path.
	boundary := atime.FromGo(time.Date(1972, 1, 1, 0, 0, 0, 0, time.UTC))
	ttBoundary := boundary.TT()
	boundaryOffset := (ttBoundary.JD() - boundary.JD()) * 86400.0
	testutil.AssertNear(t, "TT-UTC offset at 1972-01-01 (leap-second based)", boundaryOffset, 10+32.184, 1e-4)
}
