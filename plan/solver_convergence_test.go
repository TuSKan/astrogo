package plan_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

// convergenceCase is a solve whose iteration budget decides the outcome.
type convergenceCase struct {
	name    string
	maxIter int
	wantErr bool
}

// A 24-hour bracket needs about fifteen bisections to reach one second, so
// one iteration cannot converge and sixty-four comfortably can. Nothing here
// depends on the astronomy — the evaluator is a straight line, so the only
// thing under test is what the solver reports about its own budget.
func convergenceCases() []convergenceCase {
	return []convergenceCase{
		{"one iteration cannot bisect a day to a second", 1, true},
		{"three is still not enough", 3, true},
		{"the production default is ample", 64, false},
	}
}

// TestFindRootReportsNonConvergence is the whole of the defect.
//
// FindRoot used to return its current estimate with a nil error after
// exhausting MaxIter, so a caller doing the ordinary Go thing could not tell
// a converged root from an iteration budget that ran out. For an interactive
// display that is defensible; for anything that points a telescope it is not.
//
// The estimate still comes back alongside the error, which is the half that
// makes the change safe: a caller content with an approximation checks the
// sentinel and keeps the value.
func TestFindRootReportsNonConvergence(t *testing.T) {
	t.Parallel()

	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)
	t2 := t1.AddDays(1)

	// A straight line crossing zero 7.3 hours in.
	eval := plan.Evaluator(func(x time.Time) (float64, error) {
		return x.JD() - (t1.JD() + 7.3/24), nil
	})

	for _, tc := range convergenceCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := plan.Solver{Tolerance: time.Second, MaxIter: tc.maxIter}

			got, val, err := s.FindRoot(eval, t1, t2)

			switch {
			case tc.wantErr && !errors.Is(err, plan.ErrNoConvergence):
				t.Fatalf("err = %v, want ErrNoConvergence", err)
			case !tc.wantErr && err != nil:
				t.Fatalf("a converging solve reported %v", err)
			}

			// Either way the caller gets a usable estimate: that is what
			// makes the sentinel an improvement rather than a regression.
			if got.JD() < t1.JD() || got.JD() > t2.JD() {
				t.Errorf("estimate %v is outside the bracket", got)
			}

			if math.IsNaN(val) || math.IsInf(val, 0) {
				t.Errorf("estimate value is not finite: %v", val)
			}

			if !tc.wantErr {
				// And when it does converge, it converges to the root.
				wantHours := 7.3
				gotHours := (got.JD() - t1.JD()) * 24

				if math.Abs(gotHours-wantHours) > 1.0/3600 {
					t.Errorf("root at %.6f h, want %.6f h", gotHours, wantHours)
				}
			}
		})
	}
}

// TestFindExtremumReportsNonConvergence covers the same change in the other
// solver, which had the identical shape.
func TestFindExtremumReportsNonConvergence(t *testing.T) {
	t.Parallel()

	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)
	t3 := t1.AddDays(1)

	// A parabola with its maximum 7.3 hours in.
	eval := plan.Evaluator(func(x time.Time) (float64, error) {
		h := (x.JD() - t1.JD()) * 24

		return -(h - 7.3) * (h - 7.3), nil
	})

	for _, tc := range convergenceCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			s := plan.Solver{Tolerance: time.Second, MaxIter: tc.maxIter}

			got, val, err := s.FindExtremum(eval, t1, t3, true)

			switch {
			case tc.wantErr && !errors.Is(err, plan.ErrNoConvergence):
				t.Fatalf("err = %v, want ErrNoConvergence", err)
			case !tc.wantErr && err != nil:
				t.Fatalf("a converging solve reported %v", err)
			}

			if math.IsNaN(val) || math.IsInf(val, 0) {
				t.Errorf("estimate value is not finite: %v", val)
			}

			if !tc.wantErr {
				if gotHours := (got.JD() - t1.JD()) * 24; math.Abs(gotHours-7.3) > 0.01 {
					t.Errorf("maximum at %.6f h, want 7.3 h", gotHours)
				}
			}
		})
	}
}

// TestNoConvergenceNamesWhatRanOut checks the message carries enough to act
// on: a caller who sees this needs to know whether to raise MaxIter, widen
// Tolerance, or look at their evaluator.
func TestNoConvergenceNamesWhatRanOut(t *testing.T) {
	t.Parallel()

	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.LocationUTC)

	s := plan.Solver{Tolerance: time.Second, MaxIter: 1}
	eval := plan.Evaluator(func(x time.Time) (float64, error) {
		return x.JD() - (t1.JD() + 7.3/24), nil
	})

	_, _, err := s.FindRoot(eval, t1, t1.AddDays(1))
	if err == nil {
		t.Fatal("expected ErrNoConvergence")
	}

	for _, want := range []string{"1 iterations", "tolerance 1s", "bracket"} {
		if !contains(err.Error(), want) {
			t.Errorf("message %q is missing %q", err, want)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}

	return false
}
