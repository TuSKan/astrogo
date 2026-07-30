package plan_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

// nonFiniteAt returns an Evaluator that returns badVal on the callN'th
// invocation (1-indexed) and f(seconds-from-epoch) on every other call —
// used to inject a NaN/Inf evaluation at a specific point in FindRoot's or
// FindExtremum's iteration sequence without needing to predict the exact
// trial-point value the algorithm will pick internally. callN=1 lands on
// the very first evaluation (t1 for FindRoot, the midpoint for
// FindExtremum); callN=2 lands on FindRoot's t2; any higher callN lands on
// an interior trial point chosen by the algorithm during iteration.
func nonFiniteAt(f func(float64) float64, badVal float64, callN int) plan.Evaluator {
	calls := 0

	return func(t time.Time) (float64, error) {
		calls++
		if calls == callN {
			return badVal, nil
		}

		return f(t.Sub(epoch).Seconds()), nil
	}
}

// nonFiniteValues covers every IEEE 754 non-finite case: NaN comparisons
// are always false (so NaN silently survives every "!=" convergence/IQI
// gate in solver.go), and ±Inf pass the fa*fb>0 bracketing check as well
// as arithmetic like Inf-Inf, both of which historically let a
// non-finite value reach the end of FindRoot/FindExtremum with a nil
// error before this guard existed.
var nonFiniteValues = []struct {
	name string
	val  float64
}{
	{"NaN", math.NaN()},
	{"+Inf", math.Inf(1)},
	{"-Inf", math.Inf(-1)},
}

func TestFindRoot_NonFiniteEvaluation(t *testing.T) {
	s := tightSolver()

	positions := []struct {
		name  string
		callN int
	}{
		{"at_t1", 1},
		{"at_t2", 2},
		{"interior", 3},
	}

	for _, nf := range nonFiniteValues {
		for _, pos := range positions {
			t.Run(nf.name+"_"+pos.name, func(t *testing.T) {
				eval := nonFiniteAt(func(x float64) float64 { return x - 5 }, nf.val, pos.callN)

				_, _, err := s.FindRoot(eval, after(0), after(10))
				if err == nil {
					t.Fatalf("FindRoot: want error for non-finite %s at %s, got nil (non-finite value silently accepted)", nf.name, pos.name)
				}

				if !errors.Is(err, plan.ErrNonFiniteEvaluation) {
					t.Errorf("FindRoot: err = %v, want it to wrap plan.ErrNonFiniteEvaluation", err)
				}
			})
		}
	}
}

func TestFindExtremum_NonFiniteEvaluation(t *testing.T) {
	s := tightSolver()

	positions := []struct {
		name  string
		callN int
	}{
		{"at_x", 1},
		{"interior", 2},
	}

	for _, nf := range nonFiniteValues {
		for _, pos := range positions {
			t.Run(nf.name+"_"+pos.name, func(t *testing.T) {
				// f(x) = -(x-5)^2 — a maximum at x=5 over [0,10].
				eval := nonFiniteAt(func(x float64) float64 { return -(x - 5) * (x - 5) }, nf.val, pos.callN)

				_, _, err := s.FindExtremum(eval, after(0), after(10), true)
				if err == nil {
					t.Fatalf("FindExtremum: want error for non-finite %s at %s, got nil (non-finite value silently accepted)", nf.name, pos.name)
				}

				if !errors.Is(err, plan.ErrNonFiniteEvaluation) {
					t.Errorf("FindExtremum: err = %v, want it to wrap plan.ErrNonFiniteEvaluation", err)
				}
			})
		}
	}
}

// TestFindRoot_WellBehavedStillConverges is the positive control required
// alongside the non-finite guards above: a legitimate evaluator that never
// produces a non-finite value must still converge exactly as it did before
// the guards were added, confirming they reject only genuinely non-finite
// results, not merely large or extreme-but-finite ones.
func TestFindRoot_WellBehavedStillConverges(t *testing.T) {
	s := tightSolver()

	root, _, err := s.FindRoot(timeFunc(func(x float64) float64 {
		return x - 5
	}), after(0), after(10))
	if err != nil {
		t.Fatalf("FindRoot: unexpected error for a well-behaved evaluator: %v", err)
	}

	rootSec := root.Sub(epoch).Seconds()
	if math.Abs(rootSec-5) > 1e-6 {
		t.Errorf("root = %.9f, want 5.0", rootSec)
	}
}

// TestFindExtremum_WellBehavedStillConverges is FindExtremum's counterpart
// positive control.
func TestFindExtremum_WellBehavedStillConverges(t *testing.T) {
	s := tightSolver()

	tExt, val, err := s.FindExtremum(timeFunc(func(x float64) float64 {
		return -(x - 5) * (x - 5)
	}), after(0), after(10), true)
	if err != nil {
		t.Fatalf("FindExtremum: unexpected error for a well-behaved evaluator: %v", err)
	}

	extSec := tExt.Sub(epoch).Seconds()
	if math.Abs(extSec-5) > 1e-4 {
		t.Errorf("extremum time = %.6f, want ~5.0", extSec)
	}

	if math.Abs(val) > 1e-4 {
		t.Errorf("value at extremum = %g, want ~0", val)
	}
}
