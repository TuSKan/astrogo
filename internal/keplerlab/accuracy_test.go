package keplerlab_test

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

type worst struct {
	rel, abs  float64
	atE, atM  float64
	evaluated int
}

func (w *worst) add(rel, abs, e, m float64) {
	w.evaluated++

	if rel > w.rel || math.IsNaN(rel) {
		w.rel, w.atE, w.atM = rel, e, m
	}

	w.abs = math.Max(w.abs, abs)
}

// The regions the table reports. "critical" is Tommasini & Olivieri's:
// e > 0.99 with M within 0.0045 rad of perihelion, where they hold that no
// derivative-based method reaches full accuracy.
const (
	regionLow       = "e ≤ 0.5"
	regionMid       = "0.5 < e ≤ 0.99"
	regionHigh      = "e > 0.99, |M| ≥ 0.0045"
	regionCritical  = "critical: e > 0.99, |M| < 0.0045"
	regionReduction = "|M| > π (reduced)"
)

var regions = []string{regionLow, regionMid, regionHigh, regionCritical, regionReduction}

func regionOf(m, e float64) string {
	switch {
	case math.Abs(m) > math.Pi:
		return regionReduction
	case e <= 0.5:
		return regionLow
	case e <= 0.99:
		return regionMid
	case math.Abs(m) >= 0.0045:
		return regionHigh
	default:
		return regionCritical
	}
}

// TestSolverAccuracy is the measurement behind docs/kepler.md's solver
// table: every candidate against the exact root over the whole sweep. It
// asserts only the proposal's contract (see TestProposedContract); the rest
// is logged.
func TestSolverAccuracy(t *testing.T) {
	ms := meanAnomalies(testing.Short())

	results := map[string]map[string]*worst{}
	for _, s := range solvers {
		results[s.name] = map[string]*worst{}
	}

	for _, e := range eccentricities {
		for _, m := range ms {
			if m == 0 {
				continue
			}

			want := solveExact(m, e)
			r := regionOf(m, e)

			for _, s := range solvers {
				rel, abs := errorOf(s.solve(m, e), want)

				b := results[s.name][r]
				if b == nil {
					b = &worst{}
					results[s.name][r] = b
				}

				b.add(rel, abs, e, m)
			}
		}
	}

	var sb strings.Builder

	fmt.Fprintf(&sb, "\n%-18s %-34s %14s %12s  %s\n", "solver", "region", "max ulps of E", "max |ΔE| rad", "worst at")

	for _, s := range solvers {
		for _, r := range regions {
			w := results[s.name][r]
			fmt.Fprintf(&sb, "%-18s %-34s %14.3g %12.3g  e=%.17g M=%.17g (n=%d)\n",
				s.name, r, w.rel, w.abs, w.atE, w.atM, w.evaluated)
		}
	}

	t.Log(sb.String())
}

// TestZeroAndSymmetry: Kepler's equation is odd in M, so E(−M) = −E(M), and
// E(±0) = ±0. Logged per candidate as the number of sweep points at which
// either fails bit for bit.
func TestZeroAndSymmetry(t *testing.T) {
	ms := meanAnomalies(true)

	for _, s := range solvers {
		odd, zero := 0, 0

		for _, e := range eccentricities {
			for _, z := range []float64{0, math.Copysign(0, -1)} {
				if got := s.solve(z, e); got != 0 || math.Signbit(got) != math.Signbit(z) {
					zero++
				}
			}

			for _, m := range ms {
				if m <= 0 || m > math.Pi {
					continue
				}

				if a, b := s.solve(m, e), s.solve(-m, e); a != -b {
					odd++
				}
			}
		}

		t.Logf("%-18s E(±0) ≠ ±0 at %2d eccentricities; E(−M) ≠ −E(M) at %d points", s.name, zero, odd)
	}
}
