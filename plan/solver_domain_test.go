package plan_test

import (
	"errors"
	"math"
	"testing"

	"github.com/TuSKan/astrogo/plan"
	"github.com/TuSKan/astrogo/time"
)

// solverEpoch is an arbitrary fixed start; the algorithms do not care which.
func solverEpoch() time.Time {
	return time.FromGo(time.GoDate(2026, 8, 21, 0, 0, 0, 0, time.LocationUTC))
}

// atHours turns a time into hours since solverEpoch, so an evaluator can be
// written as an ordinary function of a real variable.
func atHours(t time.Time) float64 {
	return float64(t.Sub(solverEpoch())) / float64(time.Hour)
}

func plusHours(h float64) time.Time {
	return solverEpoch().Add(time.Duration(h * float64(time.Hour)))
}

// errEvaluator stands in for whatever an ephemeris lookup can fail with.
var errEvaluator = errors.New("evaluator failed")

// A bracket must be verified by the sign of the two endpoints, not by their
// product.
//
// The product test is equivalent in exact arithmetic and not in floating
// point: two same-signed values each below about 1e-162 multiply to zero, so
// an interval containing no root passes the check and the solver returns a
// confident answer from a premise it never established.
func TestFindRootRefusesAnUnbracketedInterval(t *testing.T) {
	t.Parallel()

	solver := plan.DefaultSolver()

	for _, c := range []struct {
		name   string
		fa, fb float64
	}{
		{"both positive", 3, 7},
		{"both negative", -3, -7},
		{"both tiny positive, product underflows", 1e-200, 2e-200},
		{"both tiny negative, product underflows", -1e-200, -2e-200},
		{"both denormal", 5e-324, 5e-324},
	} {
		// A straight line through the two endpoint values, so the evaluator
		// genuinely has those signs everywhere between them.
		eval := func(at time.Time) (float64, error) {
			h := atHours(at)

			return c.fa + (c.fb-c.fa)*(h/6), nil
		}

		_, _, err := solver.FindRoot(eval, solverEpoch(), plusHours(6))
		if !errors.Is(err, plan.ErrBracketingViolated) {
			t.Errorf("%s: FindRoot returned err = %v, want ErrBracketingViolated", c.name, err)
		}
	}

	// A root sitting exactly on an endpoint is bracketed, not rejected: zero
	// is neither sign.
	for _, c := range []struct {
		name   string
		fa, fb float64
	}{
		{"root at the start", 0, 5},
		{"root at the end", -5, 0},
	} {
		eval := func(at time.Time) (float64, error) {
			h := atHours(at)

			return c.fa + (c.fb-c.fa)*(h/6), nil
		}

		if _, _, err := solver.FindRoot(eval, solverEpoch(), plusHours(6)); err != nil {
			t.Errorf("%s: FindRoot refused a bracketed interval: %v", c.name, err)
		}
	}
}

// The root finder must land on the root, for functions with the shapes an
// altitude curve actually takes: nearly linear at a rise, very flat near a
// transit, and steep at a fast crossing.
func TestFindRootLandsOnKnownRoots(t *testing.T) {
	t.Parallel()

	solver := plan.Solver{Tolerance: time.Second / 1000, MaxIter: 200}

	for _, c := range []struct {
		name string
		root float64
		f    func(float64) float64
	}{
		{"linear", 3.5, func(h float64) float64 { return h - 3.5 }},
		{"cubic, flat through the root", 4, func(h float64) float64 { return (h - 4) * (h - 4) * (h - 4) }},
		{"steep exponential", 2, func(h float64) float64 { return math.Exp(h) - math.Exp(2) }},
		{"sinusoid, as an altitude curve is", 6, func(h float64) float64 { return math.Sin((h - 6) * math.Pi / 12) }},
		{"root at the very start", 0, func(h float64) float64 { return h }},
		{"root at the very end", 12, func(h float64) float64 { return h - 12 }},
	} {
		eval := func(at time.Time) (float64, error) { return c.f(atHours(at)), nil }

		got, value, err := solver.FindRoot(eval, solverEpoch(), plusHours(12))
		if err != nil {
			t.Errorf("%s: %v", c.name, err)

			continue
		}

		// One millisecond of tolerance in time, expressed in hours.
		if off := math.Abs(atHours(got) - c.root); off > 1e-6 {
			t.Errorf("%s: root found at %.9f hours, want %.9f (off by %.3g hours, f = %g)",
				c.name, atHours(got), c.root, off, value)
		}
	}
}

// A root finder that is handed a function it cannot evaluate must say so
// rather than return whichever number it last held.
func TestFindRootPropagatesFailure(t *testing.T) {
	t.Parallel()

	solver := plan.DefaultSolver()

	// Failing at the very first evaluation.
	_, _, err := solver.FindRoot(
		func(time.Time) (float64, error) { return 0, errEvaluator },
		solverEpoch(), plusHours(6))
	if !errors.Is(err, errEvaluator) {
		t.Errorf("a failing evaluator gave err = %v, want the evaluator's own error", err)
	}

	// Non-finite values must be refused at every point they can appear, not
	// silently satisfy the comparisons — every NaN comparison is false, so a
	// NaN passes convergence tests that a real value would not.
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		calls := 0
		eval := func(at time.Time) (float64, error) {
			calls++
			if calls > 2 {
				return bad, nil
			}

			return atHours(at) - 3, nil
		}

		if _, _, err := solver.FindRoot(eval, solverEpoch(), plusHours(6)); err == nil {
			t.Errorf("an evaluator returning %v mid-solve was accepted", bad)
		}
	}
}

// The extremum finder must land on the extremum, and must report the value
// unnegated whichever direction it was asked for.
func TestFindExtremumLandsOnKnownExtrema(t *testing.T) {
	t.Parallel()

	solver := plan.Solver{Tolerance: time.Second / 1000, MaxIter: 200}

	for _, c := range []struct {
		name  string
		at    float64
		value float64
		isMax bool
		f     func(float64) float64
	}{
		{"parabola maximum", 5, 9, true, func(h float64) float64 { return 9 - (h-5)*(h-5) }},
		{"parabola minimum", 7, -2, false, func(h float64) float64 { return (h-7)*(h-7) - 2 }},
		{
			// Rises from the horizon, culminates at hour 6, sets again: the
			// argument runs 0 to pi across the bracket, so the maximum is
			// interior, which is the precondition FindExtremum documents.
			// sin((h-6)*pi/12) would run -pi/2 to +pi/2 and climb the whole
			// way, putting the maximum on the endpoint with no extremum
			// inside for the method to find.
			"sinusoid maximum, the shape of a transit", 6, 1, true,
			func(h float64) float64 { return math.Sin(h * math.Pi / 12) },
		},
		{
			"quartic, very flat at the top", 4, 1, true,
			func(h float64) float64 { return 1 - math.Pow(h-4, 4)/100 },
		},
	} {
		eval := func(at time.Time) (float64, error) { return c.f(atHours(at)), nil }

		got, value, err := solver.FindExtremum(eval, solverEpoch(), plusHours(12), c.isMax)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)

			continue
		}

		// A flat extremum is poorly located in the abscissa by construction —
		// that is what flat means — so the value is the tighter check of the
		// two and the time is allowed a minute.
		if off := math.Abs(atHours(got) - c.at); off > 1.0/60 {
			t.Errorf("%s: extremum located at %.6f hours, want %.6f", c.name, atHours(got), c.at)
		}

		if off := math.Abs(value - c.value); off > 1e-6 {
			t.Errorf("%s: reported value %.9f, want %.9f — a maximisation must report the "+
				"true value, not the negated one it minimised", c.name, value, c.value)
		}
	}
}

// The cyclic crossing helpers must agree with what the quantity actually did.
func TestCrossingHelpers(t *testing.T) {
	t.Parallel()

	// A quantity that increases by a few degrees a step, which is how both
	// helpers are used: lunar elongation at about 3 degrees per six hours, and
	// solar ecliptic longitude at about one degree a day.
	for _, c := range []struct {
		name              string
		prev, cur, target float64
		want              bool
	}{
		{"crossed 90 going up", 89, 92, 90, true},
		{"did not reach 90", 86, 89, 90, false},
		{"already past 90", 92, 95, 90, false},
		{"crossed 0 by wrapping", 359, 2, 0, true},
		{"approached 0 without wrapping", 355, 358, 0, false},
		{"crossed 180", 179, 182, 180, true},
		{"crossed 270", 269, 272, 270, true},
	} {
		if got := plan.CrossesIncreasing(c.prev, c.cur, c.target, 360); got != c.want {
			t.Errorf("CrossesIncreasing(%g, %g, %g): %v, want %v", c.prev, c.cur, c.target, got, c.want)
		}

		if got := plan.CrossesTarget(c.prev, c.cur, c.target, 360); got != c.want {
			t.Errorf("CrossesTarget(%g, %g, %g): %v, want %v", c.prev, c.cur, c.target, got, c.want)
		}
	}

	// CrossesTarget also has to catch a decreasing crossing, which is what
	// distinguishes it from CrossesIncreasing.
	if !plan.CrossesTarget(92, 89, 90, 360) {
		t.Error("CrossesTarget missed a crossing of 90 from above")
	}

	if plan.CrossesIncreasing(92, 89, 90, 360) {
		t.Error("CrossesIncreasing reported a crossing for a quantity that decreased")
	}

	// A step larger than half the range is a wrap, not a crossing of every
	// target in between — the guard that keeps a wrap from being read as an
	// ordinary transit of some unrelated angle.
	if plan.CrossesTarget(10, 350, 180, 360) {
		t.Error("CrossesTarget read a wrap from 10 to 350 as a crossing of 180")
	}
}
