package plan

import (
	"fmt"
	"math"

	"github.com/TuSKan/astrogo/time"
)

// Solver provides production-grade numerical root-finding and extremum-finding
// for time-dependent astronomical functions.
//
// Root-finding uses Chandrupatla's method (1997), which combines:
//   - Inverse quadratic interpolation (IQI) for superlinear convergence
//   - Bisection as a robust fallback, selected via a simple geometric test
//   - Guaranteed bracket shrinkage on every iteration
//
// Chandrupatla's method outperforms Brent's method on "flat" functions
// (common in astronomical altitude curves near transit) and has strictly
// better worst-case behavior due to its faster fallback to bisection.
//
// Extremum-finding uses Brent's minimization (parabolic interpolation with
// golden section fallback), which is the standard for smooth unimodal functions.
//
// References:
//   - Chandrupatla, T.R. (1997). "A new hybrid quadratic/bisection algorithm
//     for finding the zero of a nonlinear function without using derivatives."
//     Advances in Engineering Software, 28(3), 145–149.
//   - Brent, R.P. (1973). Algorithms for Minimization Without Derivatives, Ch. 5.
type Solver struct {
	// Tolerance is the convergence criterion in time units.
	// The solver stops when the bracket width is smaller than this.
	Tolerance time.Duration

	// MaxIter is the maximum number of iterations before the solver returns
	// with the best approximation found so far. Typical values: 50–100.
	MaxIter int
}

// DefaultSolver returns a Solver with production defaults:
// 1-second tolerance, 64 iterations (sufficient for ~6h bracket → sub-nanosecond).
func DefaultSolver() Solver {
	return Solver{
		Tolerance: 1 * time.Second,
		MaxIter:   64,
	}
}

// Evaluator is a function that computes a real-valued metric at a given time.
// It returns the metric value and an error if the computation fails.
type Evaluator func(t time.Time) (float64, error)

// FindRoot finds the time t in [t1, t2] where eval(t) ≈ 0 using Chandrupatla's method.
//
// Precondition: eval(t1) and eval(t2) must have opposite signs (bracketing condition).
// If this is violated, FindRoot returns an error.
//
// Chandrupatla's method maintains three points {a, b, c} where:
//   - a and b bracket the root: f(a) * f(b) < 0
//   - b is the current best estimate: |f(b)| ≤ |f(a)|
//   - c is the previous iterate (third point for IQI)
//
// At each iteration, a geometric test (comparing the relative positions ξ and φ)
// determines whether inverse quadratic interpolation (IQI) through all three
// points would produce a well-conditioned step. If not, bisection is used.
// The bracket is guaranteed to shrink on every iteration.
func (s Solver) FindRoot(eval Evaluator, t1, t2 time.Time) (time.Time, float64, error) {
	// Work in float64 seconds offset from origin to avoid time.Duration precision limits.
	origin := t1
	tolSec := float64(s.Tolerance) / float64(time.Second)

	timeAt := func(sec float64) time.Time {
		return origin.Add(time.Duration(sec * float64(time.Second)))
	}

	xa := 0.0
	xb := float64(t2.Sub(t1)) / float64(time.Second)

	fa, err := eval(timeAt(xa))
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("solver: eval at a: %w", err)
	}

	fb, err := eval(timeAt(xb))
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("solver: eval at b: %w", err)
	}

	// Verify bracketing condition
	if fa*fb > 0 {
		return time.Time{}, 0, fmt.Errorf(
			"%w: f(a)=%g, f(b)=%g", ErrBracketingViolated, fa, fb)
	}

	// Third point: initialize to a (will be replaced after first iteration)
	xc, fc := xa, fa

	// Ensure |fb| ≤ |fa| — b is always the best estimate (closest to zero)
	if math.Abs(fa) < math.Abs(fb) {
		xa, xb = xb, xa
		fa, fb = fb, fa
	}

	for i := range s.MaxIter {
		// Convergence: bracket width or exact root
		if math.Abs(xb-xa) <= tolSec || fb == 0 {
			break
		}

		// ── Step selection ──────────────────────────────────────────────
		// Default: bisection (t = 0.5 means midpoint of [a, b])
		t := 0.5

		// Try IQI if the three function values are distinct (avoids division by zero)
		if fc != fa && fc != fb {
			// ξ measures how far a is from b relative to c:
			//   ξ = (a − b) / (c − b) ∈ (0, 1) when c is "behind" a
			// φ measures how the function values are distributed:
			//   φ = (fa − fb) / (fc − fb) ∈ (0, 1) for well-conditioned IQI
			xi := (xa - xb) / (xc - xb)
			phi := (fa - fb) / (fc - fb)

			// Chandrupatla's geometric test:
			// IQI is well-conditioned when the interpolation point falls inside
			// the bracket. This is guaranteed when both:
			//   φ² < ξ     (curvature from the a-side is appropriate)
			//   (1−φ)² < 1−ξ  (curvature from the b-side is appropriate)
			phi2 := phi * phi
			if phi2 < xi && (1-phi)*(1-phi) < 1-xi {
				// Inverse quadratic interpolation through (xa,fa), (xb,fb), (xc,fc).
				// Expressed as the Lagrange basis parameter t where x_new = xa + t*(xb-xa):
				//   t = L₁(0) + (c−a)/(b−a) · L₂(0)
				// where L₁, L₂ are the Lagrange basis polynomials evaluated at f=0.
				t = fa*fc/((fb-fa)*(fb-fc)) +
					(xc-xa)/(xb-xa)*fa*fb/((fc-fa)*(fc-fb))

				// Clamp to prevent stepping too close to bracket boundaries
				tlim := 0.5 * tolSec / math.Abs(xb-xa)
				if tlim < 1e-12 {
					tlim = 1e-12
				}

				t = math.Max(tlim, math.Min(1-tlim, t))
			}
		}

		// ── Evaluate at new trial point ────────────────────────────────
		xt := xa + t*(xb-xa)

		ft, err := eval(timeAt(xt))
		if err != nil {
			return time.Time{}, 0, fmt.Errorf("solver: eval at iter %d: %w", i, err)
		}

		// ── Update bracket and third point ─────────────────────────────
		if ft*fa > 0 {
			// xt is on the same side as a → a is replaced, old a becomes c
			xc, fc = xa, fa
			xa, fa = xt, ft
		} else {
			// xt is on the same side as b → b is replaced, old b becomes c
			xc, fc = xb, fb
			xb, fb = xt, ft
		}

		// Maintain invariant: |fb| ≤ |fa| — b is always the best estimate
		if math.Abs(fa) < math.Abs(fb) {
			xa, xb = xb, xa
			fa, fb = fb, fa
		}
	}

	return timeAt(xb), fb, nil
}

// FindExtremum finds the time t in [a, b] where eval(t) reaches a local
// maximum (if isMax is true) or minimum (if isMax is false).
//
// Uses Brent's method for minimization (parabolic interpolation with golden
// section fallback). The bracket [a, b] must contain a single extremum.
//
// Returns the time of the extremum and the function value at that time
// (the actual value, not negated even for maximization).
//
// Reference: Brent, R. P. (1973). Algorithms for Minimization Without Derivatives, Ch. 5.
func (s Solver) FindExtremum(eval Evaluator, t1, t3 time.Time, isMax bool) (time.Time, float64, error) {
	const goldenRatio = 0.3819660112501051 // (3 - sqrt(5)) / 2

	a, b := t1, t3
	x := a.Add(time.Duration(float64(b.Sub(a)) * 0.5))

	fx, err := eval(x)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("solver: extremum eval at x: %w", err)
	}

	if isMax {
		fx = -fx
	}

	w, v := x, x
	fw, fv := fx, fx
	e := time.Duration(0) // Distance moved on the step before last
	d := time.Duration(0) // Distance moved on the last step

	for i := range s.MaxIter {
		midpoint := a.Add(time.Duration(float64(b.Sub(a)) * 0.5))
		tol1 := float64(s.Tolerance)
		tol2 := 2.0 * tol1

		// Convergence check
		if math.Abs(float64(x.Sub(midpoint)))+float64(b.Sub(a))/2.0 <= tol2 {
			break
		}

		useParabolic := false

		if math.Abs(float64(e)) > tol1 {
			// Fit parabola through x, v, w
			xw := float64(x.Sub(w))
			xv := float64(x.Sub(v))
			r := xw * (fx - fv)
			q := xv * (fx - fw)
			p := xv*q - xw*r
			q = 2.0 * (q - r)

			if q > 0 {
				p = -p
			} else {
				q = -q
			}

			// Accept parabolic step if within bracket and reducing distance
			if math.Abs(p) < math.Abs(0.5*q*float64(e)) &&
				p > q*float64(a.Sub(x)) &&
				p < q*float64(b.Sub(x)) {
				e = d
				d = time.Duration(p / q)
				useParabolic = true
			}
		}

		if !useParabolic {
			// Golden section step (robust fallback)
			if x.After(midpoint) || x.Equal(midpoint) {
				e = a.Sub(x)
			} else {
				e = b.Sub(x)
			}

			d = time.Duration(float64(e) * goldenRatio)
		}

		// Evaluate at the new trial point
		var u time.Time

		switch {
		case math.Abs(float64(d)) >= tol1:
			u = x.Add(d)
		case float64(d) > 0:
			u = x.Add(time.Duration(tol1))
		default:
			u = x.Add(time.Duration(-tol1))
		}

		fu, err := eval(u)
		if err != nil {
			return time.Time{}, 0, fmt.Errorf("solver: extremum eval at iter %d: %w", i, err)
		}

		if isMax {
			fu = -fu
		}

		// Update bracket
		if fu <= fx {
			if u.Before(x) {
				b = x
			} else {
				a = x
			}

			v, w, x = w, x, u
			fv, fw, fx = fw, fx, fu
		} else {
			if u.Before(x) {
				a = u
			} else {
				b = u
			}

			if fu <= fw || w.Equal(x) {
				v, w = w, u
				fv, fw = fw, fu
			} else if fu <= fv || v.Equal(x) || v.Equal(w) {
				v = u
				fv = fu
			}
		}
	}

	// Return the actual (non-negated) function value
	finalVal, err := eval(x)
	if err != nil {
		return time.Time{}, 0, fmt.Errorf("solver: final eval: %w", err)
	}

	return x, finalVal, err
}

// CrossesTarget checks whether a cyclic quantity moved through a target angle
// in one step. Handles the wrap-around at wrapAt (e.g., 360 for degrees).
//
// For example, to check if Moon-Sun elongation crossed 90°:
//
//	CrossesTarget(prevElong, curElong, 90, 360)
func CrossesTarget(prev, cur, target, wrapAt float64) bool {
	halfWrap := wrapAt / 2.0

	// Direct crossing (no wraparound)
	if (prev <= target && cur > target) || (prev > target && cur <= target) {
		if math.Abs(cur-prev) < halfWrap {
			return true
		}
	}

	// Handle wraparound at 0/wrapAt boundary
	if target == 0 {
		if prev > wrapAt-halfWrap && cur < halfWrap {
			return true
		}
	}

	return false
}

// CrossesIncreasing checks whether a monotonically increasing (with wrap)
// quantity crossed a target value. Used for Sun's ecliptic longitude (~1°/day).
//
// The wrap detection uses a half-wrap window: a wraparound is detected when
// prev is in the upper half of the range and cur is in the lower half,
// ensuring correctness regardless of step size.
func CrossesIncreasing(prev, cur, target, wrapAt float64) bool {
	// Normal monotonic crossing
	if prev <= target && cur > target {
		return true
	}
	// Handle wraparound (e.g., 359° → 1° crossing 0°)
	halfWrap := wrapAt / 2.0
	if target == 0 && prev > halfWrap && cur < halfWrap {
		return true
	}

	return false
}
