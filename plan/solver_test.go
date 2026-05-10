package plan_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

// ── Helpers ──────────────────────────────────────────────────────────────────

// epoch is a fixed reference time for test evaluators.
var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)

// timeFunc wraps a pure mathematical function f(seconds) into an Evaluator
// where seconds is measured from epoch.
func timeFunc(f func(float64) float64) plan.Evaluator {
	return func(t time.Time) (float64, error) {
		sec := t.Sub(epoch).Seconds()
		return f(sec), nil
	}
}

// after returns epoch + d seconds.
func after(seconds float64) time.Time {
	return epoch.Add(time.Duration(seconds * float64(time.Second)))
}

// tightSolver returns a solver with microsecond tolerance for analytical tests.
func tightSolver() plan.Solver {
	return plan.Solver{
		Tolerance: time.Second / 1e6, // 1 µs
		MaxIter:   100,
	}
}

// ── FindRoot (Chandrupatla) Tests ────────────────────────────────────────────

func TestFindRoot_LinearFunction(t *testing.T) {
	// f(x) = x - 5 → root at x = 5
	s := tightSolver()

	root, fval, err := s.FindRoot(timeFunc(func(x float64) float64 {
		return x - 5
	}), after(0), after(10))
	if err != nil {
		t.Fatalf("FindRoot failed: %v", err)
	}

	rootSec := root.Sub(epoch).Seconds()
	t.Logf("root=%.9f s  f(root)=%e", rootSec, fval)

	if math.Abs(rootSec-5) > 1e-6 {
		t.Errorf("root=%.9f, want 5.0 (Δ=%e)", rootSec, rootSec-5)
	}
}

func TestFindRoot_QuadraticFunction(t *testing.T) {
	// f(x) = x² - 4 → roots at x = ±2; bracket [0, 10] finds x = 2
	s := tightSolver()

	root, fval, err := s.FindRoot(timeFunc(func(x float64) float64 {
		return x*x - 4
	}), after(0), after(10))
	if err != nil {
		t.Fatalf("FindRoot failed: %v", err)
	}

	rootSec := root.Sub(epoch).Seconds()
	t.Logf("root=%.9f s  f(root)=%e", rootSec, fval)

	if math.Abs(rootSec-2) > 1e-6 {
		t.Errorf("root=%.9f, want 2.0 (Δ=%e)", rootSec, rootSec-2)
	}
}

func TestFindRoot_SineFunction(t *testing.T) {
	// f(x) = sin(x) → root at x = π ≈ 3.14159..., bracket [2, 4]
	s := tightSolver()

	root, fval, err := s.FindRoot(timeFunc(func(x float64) float64 {
		return math.Sin(x)
	}), after(2), after(4))
	if err != nil {
		t.Fatalf("FindRoot failed: %v", err)
	}

	rootSec := root.Sub(epoch).Seconds()
	t.Logf("root=%.12f s  f(root)=%e  (π=%.12f)", rootSec, fval, math.Pi)

	if math.Abs(rootSec-math.Pi) > 1e-6 {
		t.Errorf("root=%.12f, want π=%.12f (Δ=%e)", rootSec, math.Pi, rootSec-math.Pi)
	}
}

func TestFindRoot_FlatFunction(t *testing.T) {
	// f(x) = (x - 3)³ — has a triple root at x = 3, very flat there.
	// This is where Chandrupatla excels over Brent's method.
	s := tightSolver()

	root, fval, err := s.FindRoot(timeFunc(func(x float64) float64 {
		d := x - 3
		return d * d * d
	}), after(0), after(6))
	if err != nil {
		t.Fatalf("FindRoot failed: %v", err)
	}

	rootSec := root.Sub(epoch).Seconds()
	t.Logf("root=%.9f s  f(root)=%e (flat triple root)", rootSec, fval)

	if math.Abs(rootSec-3) > 1e-3 {
		t.Errorf("root=%.9f, want 3.0 (Δ=%e)", rootSec, rootSec-3)
	}
}

func TestFindRoot_ExponentialDecay(t *testing.T) {
	// f(x) = exp(-x) - 0.01 → root at x = ln(100) ≈ 4.60517...
	s := tightSolver()
	expected := math.Log(100)

	root, fval, err := s.FindRoot(timeFunc(func(x float64) float64 {
		return math.Exp(-x) - 0.01
	}), after(0), after(10))
	if err != nil {
		t.Fatalf("FindRoot failed: %v", err)
	}

	rootSec := root.Sub(epoch).Seconds()
	t.Logf("root=%.9f s  f(root)=%e  (expected=%.9f)", rootSec, fval, expected)

	if math.Abs(rootSec-expected) > 1e-6 {
		t.Errorf("root=%.9f, want %.9f (Δ=%e)", rootSec, expected, rootSec-expected)
	}
}

func TestFindRoot_AstronomicalScale(t *testing.T) {
	// Simulate altitude crossing zero 8 hours (28800 s) into a 24-hour bracket.
	// f(t) = sin(2π·t / 86400 - π/3) — crosses zero at t = 86400/6 = 14400 s
	// and at t = 86400/2 + 14400 = 57600 s
	s := plan.DefaultSolver() // 1-second tolerance
	period := 86400.0

	root, fval, err := s.FindRoot(timeFunc(func(x float64) float64 {
		return math.Sin(2*math.Pi*x/period - math.Pi/3)
	}), after(10000), after(20000))
	if err != nil {
		t.Fatalf("FindRoot failed: %v", err)
	}

	rootSec := root.Sub(epoch).Seconds()
	expected := period / 6 // = 14400
	t.Logf("root=%.3f s  f(root)=%e  (expected=%.1f s)", rootSec, fval, expected)

	if math.Abs(rootSec-expected) > 1.0 {
		t.Errorf("root=%.3f, want %.1f (Δ=%.3f s)", rootSec, expected, rootSec-expected)
	}
}

func TestFindRoot_BracketingViolation(t *testing.T) {
	// f(x) = x² + 1 is always positive → no root → bracketing error
	s := tightSolver()

	_, _, err := s.FindRoot(timeFunc(func(x float64) float64 {
		return x*x + 1
	}), after(1), after(10))
	if err == nil {
		t.Fatal("expected bracketing violation error, got nil")
	}

	t.Logf("correctly returned error: %v", err)
}

func TestFindRoot_ExactRootAtEndpoint(t *testing.T) {
	// f(x) = x → root at x = 0, which is the left endpoint
	s := tightSolver()

	root, fval, err := s.FindRoot(timeFunc(func(x float64) float64 {
		return x
	}), after(0), after(10))
	if err != nil {
		t.Fatalf("FindRoot failed: %v", err)
	}

	rootSec := root.Sub(epoch).Seconds()
	t.Logf("root=%.9f s  f(root)=%e", rootSec, fval)

	// The solver should converge very close to 0
	if math.Abs(rootSec) > 1e-3 {
		t.Errorf("root=%.9f, want 0.0 (Δ=%e)", rootSec, rootSec)
	}
}

// ── FindExtremum (Brent's minimization) Tests ────────────────────────────────

func TestFindExtremum_QuadraticMinimum(t *testing.T) {
	// f(x) = (x - 7)² → minimum at x = 7
	s := tightSolver()

	minT, minVal, err := s.FindExtremum(timeFunc(func(x float64) float64 {
		d := x - 7
		return d * d
	}), after(0), after(14), false)
	if err != nil {
		t.Fatalf("FindExtremum failed: %v", err)
	}

	minSec := minT.Sub(epoch).Seconds()
	t.Logf("min=%.9f s  f(min)=%e", minSec, minVal)

	if math.Abs(minSec-7) > 1e-3 {
		t.Errorf("min=%.9f, want 7.0 (Δ=%e)", minSec, minSec-7)
	}

	if minVal > 1e-6 {
		t.Errorf("f(min)=%e, want ≈0", minVal)
	}
}

func TestFindExtremum_QuadraticMaximum(t *testing.T) {
	// f(x) = -(x - 5)² + 100 → maximum at x = 5, f(5) = 100
	s := tightSolver()

	maxT, maxVal, err := s.FindExtremum(timeFunc(func(x float64) float64 {
		d := x - 5
		return -d*d + 100
	}), after(0), after(10), true)
	if err != nil {
		t.Fatalf("FindExtremum failed: %v", err)
	}

	maxSec := maxT.Sub(epoch).Seconds()
	t.Logf("max=%.9f s  f(max)=%.6f", maxSec, maxVal)

	if math.Abs(maxSec-5) > 1e-3 {
		t.Errorf("max=%.9f, want 5.0 (Δ=%e)", maxSec, maxSec-5)
	}

	if math.Abs(maxVal-100) > 0.01 {
		t.Errorf("f(max)=%.6f, want 100.0", maxVal)
	}
}

func TestFindExtremum_CosineMinimum(t *testing.T) {
	// f(x) = cos(x) → minimum at x = π ≈ 3.14159...
	s := tightSolver()

	minT, minVal, err := s.FindExtremum(timeFunc(func(x float64) float64 {
		return math.Cos(x)
	}), after(2), after(4), false)
	if err != nil {
		t.Fatalf("FindExtremum failed: %v", err)
	}

	minSec := minT.Sub(epoch).Seconds()
	t.Logf("min=%.12f s  f(min)=%.12f  (π=%.12f)", minSec, minVal, math.Pi)

	if math.Abs(minSec-math.Pi) > 1e-3 {
		t.Errorf("min=%.12f, want π=%.12f (Δ=%e)", minSec, math.Pi, minSec-math.Pi)
	}

	if math.Abs(minVal-(-1)) > 0.001 {
		t.Errorf("f(min)=%.12f, want -1.0", minVal)
	}
}

func TestFindExtremum_TransitSimulation(t *testing.T) {
	// Simulate a transit altitude curve: altitude peaks 6 hours into a 12-hour window.
	// f(t) = -cos(2π·t / 43200) → peaks at t = 21600 s (6 hours)
	s := plan.DefaultSolver()
	period := 43200.0

	maxT, _, err := s.FindExtremum(timeFunc(func(x float64) float64 {
		return -math.Cos(2 * math.Pi * x / period)
	}), after(0), after(period), true)
	if err != nil {
		t.Fatalf("FindExtremum failed: %v", err)
	}

	maxSec := maxT.Sub(epoch).Seconds()
	expected := period / 2.0 // 21600 s
	t.Logf("transit=%.3f s (%.1f h)  expected=%.1f s (%.1f h)", maxSec, maxSec/3600, expected, expected/3600)

	if math.Abs(maxSec-expected) > 1.0 {
		t.Errorf("transit=%.3f s, want %.1f s (Δ=%.3f s)", maxSec, expected, maxSec-expected)
	}
}

// ── CrossesTarget / CrossesIncreasing Tests ──────────────────────────────────

func TestCrossesTarget(t *testing.T) {
	tests := []struct {
		name   string
		prev   float64
		cur    float64
		target float64
		wrapAt float64
		want   bool
	}{
		{"simple upward", 85, 95, 90, 360, true},
		{"simple downward", 95, 85, 90, 360, true},
		{"no crossing", 85, 89, 90, 360, false},
		{"wraparound at 0", 359, 1, 0, 360, true},
		{"far from target", 100, 200, 90, 360, false},
		{"exact at prev", 90, 91, 90, 360, true}, // prev==target, crosses upward
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := plan.CrossesTarget(tc.prev, tc.cur, tc.target, tc.wrapAt)
			if got != tc.want {
				t.Errorf("CrossesTarget(%.1f, %.1f, %.1f, %.1f) = %v, want %v",
					tc.prev, tc.cur, tc.target, tc.wrapAt, got, tc.want)
			}
		})
	}
}

func TestCrossesIncreasing(t *testing.T) {
	tests := []struct {
		name   string
		prev   float64
		cur    float64
		target float64
		wrapAt float64
		want   bool
	}{
		{"normal crossing", 85, 95, 90, 360, true},
		{"decreasing (not increasing)", 95, 85, 90, 360, false},
		{"no crossing", 85, 89, 90, 360, false},
		{"wrap at 0", 359, 1, 0, 360, true},
		{"not at wrap", 10, 20, 0, 360, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := plan.CrossesIncreasing(tc.prev, tc.cur, tc.target, tc.wrapAt)
			if got != tc.want {
				t.Errorf("CrossesIncreasing(%.1f, %.1f, %.1f, %.1f) = %v, want %v",
					tc.prev, tc.cur, tc.target, tc.wrapAt, got, tc.want)
			}
		})
	}
}

// ── Convergence Rate Test ────────────────────────────────────────────────────

func TestFindRoot_ConvergenceCount(t *testing.T) {
	// Count iterations for a smooth function to verify superlinear convergence.
	// f(x) = x² - 2 → root at sqrt(2) ≈ 1.41421...
	// With Chandrupatla's IQI, this should converge in well under 20 iterations
	// for microsecond tolerance from a [0, 10] bracket.
	count := 0
	s := plan.Solver{Tolerance: time.Second / 1e6, MaxIter: 100} // 1 µs

	root, _, err := s.FindRoot(func(t time.Time) (float64, error) {
		x := t.Sub(epoch).Seconds()
		count++

		return x*x - 2, nil
	}, after(0.1), after(10))
	if err != nil {
		t.Fatalf("FindRoot failed: %v", err)
	}

	rootSec := root.Sub(epoch).Seconds()
	t.Logf("root=%.12f (√2=%.12f)  evaluations=%d", rootSec, math.Sqrt(2), count)

	if count > 30 {
		t.Errorf("too many evaluations: %d (expected <30 for Chandrupatla on smooth quadratic)", count)
	}

	if math.Abs(rootSec-math.Sqrt(2)) > 1e-6 {
		t.Errorf("root=%.12f, want √2=%.12f", rootSec, math.Sqrt(2))
	}
}

// ── DefaultSolver Tests ──────────────────────────────────────────────────────

func TestDefaultSolver(t *testing.T) {
	s := plan.DefaultSolver()
	if s.Tolerance != time.Second {
		t.Errorf("Tolerance=%v, want 1s", s.Tolerance)
	}

	if s.MaxIter != 64 {
		t.Errorf("MaxIter=%d, want 64", s.MaxIter)
	}
}
