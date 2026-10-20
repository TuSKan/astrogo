package keplerlab_test

import (
	"fmt"
	"math"
	"math/big"
	"strings"
	"testing"
)

// eccentricities spans the elliptic range up to its last float64, densest
// where the equation's conditioning, 1/(1 − e·cos E), is worst.
var eccentricities = []float64{
	0, 1e-3, 0.1, 0.5, 0.7, 0.9, 0.99, 0.999, 0.99999,
	1 - 1e-7, 1 - 1e-10, 1 - 1e-13, math.Nextafter(1, 0),
}

// meanAnomalies is the sweep: log-spaced from 10⁻²⁰ to π, both signs, the
// float64s on either side of whole revolutions, and phases many revolutions
// out. Under -short it is thinned, not dropped.
func meanAnomalies(short bool) []float64 {
	perDecade := 12
	if short {
		perDecade = 2
	}

	var ms []float64

	for k := -20 * perDecade; k <= 0; k++ {
		m := math.Pi * math.Pow(10, float64(k)/float64(perDecade))
		ms = append(ms, m, -m)
	}

	for _, rev := range []float64{1, 7, 1e3, 1e6} {
		c := rev * 2 * math.Pi
		ms = append(ms, c, math.Nextafter(c, 0), math.Nextafter(c, math.Inf(1)), c+1e-9, c-1e-9, c+1.5, -c-0.5)
	}

	return append(ms, 0, math.Copysign(0, -1), 1e9, 1e12)
}

// errorOf is a candidate's error against the exact root: relative, in ulps of
// E, and absolute, in radians. The difference is taken modulo 2π, so an
// answer on another branch — [0, 2π) rather than [−π, π] — is judged as the
// angle it is, through the float64 it is: its cos and sin are what a state is
// built from.
func errorOf(got float64, want *big.Float) (rel, abs float64) {
	if math.IsNaN(got) || math.IsInf(got, 0) {
		return math.Inf(1), math.Inf(1)
	}

	d := new(big.Float).SetPrec(reducePrec).SetFloat64(got)
	d.Sub(d, want)

	k := new(big.Float).SetPrec(reducePrec).Quo(d, big2Pi)
	if kf, _ := k.Float64(); math.Abs(kf) >= 0.5 {
		d.Sub(d, new(big.Float).SetPrec(reducePrec).Mul(big2Pi, new(big.Float).SetFloat64(math.Round(kf))))
	}

	abs, _ = d.Float64()
	abs = math.Abs(abs)

	if want.Sign() == 0 {
		return 0, abs // M = ±0 is judged by TestZeroAndSymmetry
	}

	d.Quo(d, want)
	rel, _ = d.Float64()

	return math.Abs(rel) / 0x1p-52, abs
}

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
