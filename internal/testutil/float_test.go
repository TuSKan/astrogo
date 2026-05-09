package testutil_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// ── InAbsTol ─────────────────────────────────────────────────────────────────

func TestInAbsTol(t *testing.T) {
	cases := []struct {
		name string
		got  float64
		want float64
		tol  float64
		ok   bool
	}{
		{"equal", 1.0, 1.0, 0.0, true},
		{"within tol", 1.125, 1.0, 0.25, true},    // 0.125 < 0.25, exact in binary
		{"exact boundary", 1.25, 1.0, 0.25, true}, // 0.25 exact in binary, diff == tol
		{"just outside", 1.5, 1.0, 0.25, false},   // 0.5 > 0.25
		{"negative diff", 0.75, 1.0, 0.25, true},
		{"negative tol rejects", 1.0, 1.0, -1e-9, false},
		{"NaN got", math.NaN(), 1.0, 0.01, false},
		{"NaN want", 1.0, math.NaN(), 0.01, false},
		{"Inf equal", math.Inf(1), math.Inf(1), 0, false}, // Inf-Inf = NaN
	}
	for i, c := range cases {
		if got := testutil.InAbsTol(c.got, c.want, c.tol); got != c.ok {
			t.Errorf("%s: InAbsTol(%v, %v, %v) = %v, want %v",
				testutil.CaseLabel(i, c.name), c.got, c.want, c.tol, got, c.ok)
		}
	}
}

// ── InRelTol ─────────────────────────────────────────────────────────────────

func TestInRelTol(t *testing.T) {
	cases := []struct {
		name   string
		got    float64
		want   float64
		relTol float64
		ok     bool
	}{
		{"exact", 1.0, 1.0, 0.0, true},
		{"25% error within 50%", 1.25, 1.0, 0.5, true}, // exact binary values
		{"exact boundary 25%", 1.25, 1.0, 0.25, true},  // diff=0.25, want=1.0, rel=0.25 exactly
		{"25% error exceeds 10%", 1.25, 1.0, 0.1, false},
		{"want zero, got zero", 0.0, 0.0, 1e-9, true},
		{"want zero, got 1e-10 within 1e-9", 1e-10, 0.0, 1e-9, true},
		{"want zero, got 1e-8 exceeds 1e-9", 1e-8, 0.0, 1e-9, false},
		{"negative tol rejects", 1.0, 1.0, -0.01, false},
		{"large relative error", 2.0, 1.0, 0.5, false}, // 100% error
		{"large tol allows", 2.0, 1.0, 1.0, true},
	}
	for i, c := range cases {
		if got := testutil.InRelTol(c.got, c.want, c.relTol); got != c.ok {
			t.Errorf("%s: InRelTol(%v, %v, %v) = %v, want %v",
				testutil.CaseLabel(i, c.name), c.got, c.want, c.relTol, got, c.ok)
		}
	}
}

// ── InAngleTol ───────────────────────────────────────────────────────────────

func TestInAngleTol(t *testing.T) {
	const arcsec = math.Pi / (180 * 3600)
	const deg = math.Pi / 180
	cases := []struct {
		name string
		got  float64
		want float64
		tol  float64
		ok   bool
	}{
		{"identical", 1.0, 1.0, 1e-15, true},
		{"within tol", 1.0 + 1e-6, 1.0, 1e-5, true},
		{"exactly at tol", 0.0 + arcsec, 0.0, arcsec, true},
		{"outside tol", 1.0 + 1e-5, 1.0, 1e-6, false},
		// Wraparound cases
		{"0 vs 2π same", 0.0, 2 * math.Pi, 1e-12, true},
		{"2π vs 4π same", 2 * math.Pi, 4 * math.Pi, 1e-12, true},
		{"near-zero and near-2π", 0.001, 2*math.Pi - 0.001, 0.003, true},
		{"near-zero and near-2π outside", 0.001, 2*math.Pi - 0.001, 0.001, false},
		// Opposite sides of sky (π apart) are far
		{"180 deg apart", 0.0, math.Pi, 1 * deg, false},
		{"negative tol rejects", 0.0, 0.0, -1.0, false},
	}
	for i, c := range cases {
		if got := testutil.InAngleTol(c.got, c.want, c.tol); got != c.ok {
			t.Errorf("%s: InAngleTol(%v, %v, %v) = %v, want %v",
				testutil.CaseLabel(i, c.name), c.got, c.want, c.tol, got, c.ok)
		}
	}
}

// ── Assertion wrappers (passing cases only) ───────────────────────────────────
// The failure paths are guaranteed correct by the predicate tests above;
// assertion wrappers just call t.Errorf when the predicate returns false.

func TestAssertNear_passes(t *testing.T) {
	testutil.AssertNear(t, "identity", 1.0, 1.0, 0)
	testutil.AssertNear(t, "within", 1.001, 1.0, 0.01)
}

func TestAssertRelNear_passes(t *testing.T) {
	testutil.AssertRelNear(t, "identity", 1.0, 1.0, 0)
	testutil.AssertRelNear(t, "1percent", 1.01, 1.0, 0.02)
}

func TestAssertAngleNear_passes(t *testing.T) {
	testutil.AssertAngleNear(t, "identity", 1.5, 1.5, 0)
	testutil.AssertAngleNear(t, "wraparound", 0.0, 2*math.Pi, 1e-12)
}

func TestAssertExact_passes(t *testing.T) {
	testutil.AssertExact(t, "integer JD", 2451545.0, 2451545.0)
	testutil.AssertExact(t, "zero", 0.0, 0.0)
}

// ── Failure detection via spy ─────────────────────────────────────────────────
// A minimal spy implements testing.TB's Errorf to record failures without
// stopping the parent test.

type tbSpy struct {
	testing.TB
	msg    string
	failed bool
}

func (s *tbSpy) Helper() {}
func (s *tbSpy) Errorf(format string, args ...any) {
	s.failed = true
	s.msg = format // enough to verify the code path was taken
}
func (s *tbSpy) Logf(string, ...any) {}

func TestAssertNear_fails(t *testing.T) {
	spy := &tbSpy{TB: t}
	testutil.AssertNear(spy, "x", 2.0, 1.0, 0.001)
	if !spy.failed {
		t.Errorf("AssertNear did not fail for clearly different values")
	}
}

func TestAssertRelNear_fails(t *testing.T) {
	spy := &tbSpy{TB: t}
	testutil.AssertRelNear(spy, "x", 2.0, 1.0, 0.01)
	if !spy.failed {
		t.Errorf("AssertRelNear did not fail for 100%% relative error")
	}
}

func TestAssertAngleNear_fails(t *testing.T) {
	spy := &tbSpy{TB: t}
	testutil.AssertAngleNear(spy, "x", 0.0, math.Pi, 0.001)
	if !spy.failed {
		t.Errorf("AssertAngleNear did not fail for π-apart angles")
	}
}

func TestAssertExact_fails(t *testing.T) {
	spy := &tbSpy{TB: t}
	testutil.AssertExact(spy, "x", 1.0, 1.0+1e-15)
	if !spy.failed {
		t.Errorf("AssertExact did not fail for differing values")
	}
}

// ── DegToRad / RadToDeg round-trip ────────────────────────────────────────────

func TestDegRadRoundTrip(t *testing.T) {
	cases := []float64{0, 45, 90, 180, 270, 360, -90, -180}
	for _, deg := range cases {
		got := testutil.RadToDeg(testutil.DegToRad(deg))
		testutil.AssertNear(t, "round-trip", got, deg, 1e-12)
	}
}
