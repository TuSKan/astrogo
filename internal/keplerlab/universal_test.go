package keplerlab_test

import (
	"fmt"
	"math"
	"math/big"
	"sort"
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/internal/keplerlab"
)

// gaussMu is k², the Sun's GM in AU³/day² with Gauss's constant.
const gaussMu = 0.01720209895 * 0.01720209895

// prodStumpff is ephemeris/kepler's stumpff as it shipped until #833: the
// Maclaurin series below |z| = 0.1, the closed forms above. Kept so the
// "before" column of docs/kepler.md §7 reproduces.
func prodStumpff(z float64) (c, s float64) {
	if math.Abs(z) < 0.1 {
		tc, ts := 0.5, 1.0/6
		c, s = tc, ts

		for k := 1; k < 20; k++ {
			tc *= -z / float64((2*k+1)*(2*k+2))
			ts *= -z / float64((2*k+2)*(2*k+3))
			c += tc
			s += ts
		}

		return c, s
	}

	if z > 0 {
		sz := math.Sqrt(z)

		return (1 - math.Cos(sz)) / z, (sz - math.Sin(sz)) / (sz * sz * sz)
	}

	sz := math.Sqrt(-z)

	return (math.Cosh(sz) - 1) / -z, (math.Sinh(sz) - sz) / (sz * sz * sz)
}

// candStumpff is c₀ … c₃ with no subtraction of nearly equal quantities: the
// series below |z| = 4, where the closed form of c₃ still loses a bit or two,
// and above it the half-angle form 2 sin²(x/2)/x² for c₂. It is what #833
// shipped as ephemeris/kepler's stumpff.
func candStumpff(z float64) (c0, c1, c2, c3 float64) {
	if math.Abs(z) < 4 {
		t0, t1, t2, t3 := 1.0, 1.0, 0.5, 1.0/6
		c0, c1, c2, c3 = t0, t1, t2, t3

		for j := 1; j < 30; j++ {
			fj := float64(j)
			t0 *= -z / ((2*fj - 1) * (2 * fj))
			t1 *= -z / ((2 * fj) * (2*fj + 1))
			t2 *= -z / ((2*fj + 1) * (2*fj + 2))
			t3 *= -z / ((2*fj + 2) * (2*fj + 3))
			c0, c1, c2, c3 = c0+t0, c1+t1, c2+t2, c3+t3

			if math.Abs(t0) < 0x1p-60*math.Abs(c0) && math.Abs(t3) < 0x1p-60*c3 {
				break
			}
		}

		return c0, c1, c2, c3
	}

	if z > 0 {
		x := math.Sqrt(z)
		s, c := math.Sincos(x)
		h := math.Sin(x / 2)

		return c, s / x, 2 * h * h / (x * x), (x - s) / (x * x * x)
	}

	x := math.Sqrt(-z)
	h := math.Sinh(x / 2)
	sh := math.Sinh(x)

	return math.Cosh(x), sh / x, 2 * h * h / (x * x), (sh - x) / (x * x * x)
}

// prodSolve is ephemeris/kepler's solveUniversal, over a Stumpff pair: the
// solve itself did not change in #833, only the functions it is given.
func prodSolve(q, e, alpha, sqrtMu, dt float64, stumpff func(float64) (float64, float64)) float64 {
	const (
		tolerance = 1e-13
		maxIter   = 200
	)

	target := sqrtMu * math.Abs(dt)
	if target == 0 {
		return 0
	}

	lo, hi := 0.0, target/q
	chi := hi

	switch {
	case alpha > 0:
		chi = target * alpha
	case alpha < 0:
		sqrtNegA := math.Sqrt(-1 / alpha)
		chi = sqrtNegA * math.Asinh(target*(-alpha)/sqrtNegA/e)
	}

	if !(chi > lo && chi <= hi) {
		chi = hi
	}

	for range maxIter {
		c, s := stumpff(alpha * chi * chi)
		f := e*chi*chi*chi*s + q*chi - target
		fp := e*chi*chi*c + q

		if f < 0 {
			lo = chi
		} else {
			hi = chi
		}

		step := f / fp
		next := chi - step

		if math.Abs(step) <= tolerance*math.Max(1, math.Abs(chi)) && !math.IsNaN(next) && !math.IsInf(next, 0) {
			return math.Copysign(next, dt)
		}

		if !(next > lo && next < hi) {
			next = lo + (hi-lo)/2
		}

		chi = next
	}

	return math.NaN()
}

// prodState is perifocalUniversal as it shipped until #833, without the
// fold: f and g as 1 − χ²C/q, Δt − χ³S/√μ, and their derivatives as written
// there.
func prodState(q, e, mu, dt float64) (chi, x, y, vx, vy float64) {
	alpha := (1 - e) / q
	sqrtMu := math.Sqrt(mu)

	chi = prodSolve(q, e, alpha, sqrtMu, dt, prodStumpff)

	c, s := prodStumpff(alpha * chi * chi)
	r := q + e*chi*chi*c
	v0 := math.Sqrt(mu * (1 + e) / q)

	f := 1 - chi*chi*c/q
	g := dt - chi*chi*chi*s/sqrtMu
	fDot := sqrtMu / (r * q) * (alpha*chi*chi*chi*s - chi)
	gDot := 1 - chi*chi*c/r

	return chi, f * q, g * v0, fDot * q, gDot * v0
}

// candState is the candidate #833 shipped: the same solve over candStumpff,
// and the state through c₀ and c₁, the identities 1 − z·c₃ = c₁ and
// 1 − z·c₂ = c₀ taking the cancellation out of g and ġ:
//
//	x = q − χ²c₂,  y = v₀·q·χ·c₁/√μ,  vx = −√μ·χ·c₁/r,  vy = v₀·q·c₀/r.
func candState(q, e, mu, dt float64) (chi, x, y, vx, vy float64) {
	alpha := (1 - e) / q
	sqrtMu := math.Sqrt(mu)

	cs := func(z float64) (float64, float64) {
		_, _, c2, c3 := candStumpff(z)

		return c2, c3
	}

	chi = prodSolve(q, e, alpha, sqrtMu, dt, cs)

	c0, c1, c2, _ := candStumpff(alpha * chi * chi)
	r := q + e*chi*chi*c2
	v0 := math.Sqrt(mu * (1 + e) / q)

	return chi, q - chi*chi*c2, v0 * q * chi * c1 / sqrtMu, -sqrtMu * chi * c1 / r, v0 * q * c0 / r
}

// mixState is the shipped f and g over the candidate's Stumpff functions:
// what the Stumpff fix alone buys.
func mixState(q, e, mu, dt float64) (chi, x, y, vx, vy float64) {
	alpha := (1 - e) / q
	sqrtMu := math.Sqrt(mu)

	cs := func(z float64) (float64, float64) {
		_, _, c2, c3 := candStumpff(z)

		return c2, c3
	}

	chi = prodSolve(q, e, alpha, sqrtMu, dt, cs)

	c, s := cs(alpha * chi * chi)
	r := q + e*chi*chi*c
	v0 := math.Sqrt(mu * (1 + e) / q)

	f := 1 - chi*chi*c/q
	g := dt - chi*chi*chi*s/sqrtMu
	fDot := sqrtMu / (r * q) * (alpha*chi*chi*chi*s - chi)
	gDot := 1 - chi*chi*c/r

	return chi, f * q, g * v0, fDot * q, gDot * v0
}

// ulpsOf is a float64's error against a Prec value, in ulps of the value.
func ulpsOf(got float64, want *big.Float) float64 {
	if want.Sign() == 0 {
		return math.Abs(got) / 0x1p-1074
	}

	d := new(big.Float).SetPrec(prec).Sub(newF(got), want)
	d.Quo(d, want)
	f, _ := d.Float64()

	return math.Abs(f) / 0x1p-52
}

// ulpsOfAtLeastOne is a float64's error against a Prec value, in ulps of
// max(|value|, 1): for a function that crosses zero.
func ulpsOfAtLeastOne(got float64, want *big.Float) float64 {
	d := new(big.Float).SetPrec(prec).Sub(newF(got), want)
	f, _ := d.Float64()

	return math.Abs(f) / math.Max(math.Abs(mustF(want)), 1) / 0x1p-52
}

// TestOracleStumpff: the series against the identities c₀ = 1 − z·c₂,
// c₁ = 1 − z·c₃ and c₀² + z·c₁² = 1, and against math.Cos and math.Sinh
// at points where those are accurate.
func TestOracleStumpff(t *testing.T) {
	for _, z := range []float64{1e-30, -1e-30, 0.1, -0.1, 1, -1, 9.8, -40, -400} {
		c := keplerlab.Stumpff(z)
		bz := newF(z)

		// Each identity is a difference of terms as large as c₀², which is
		// cosh²√−z on a hyperbola: its rounding at Prec is the scale.
		scale := max(c[0].MantExp(nil)*2, 1)

		check := func(name string, got, want *big.Float) {
			d := new(big.Float).SetPrec(prec).Sub(got, want)
			if d.Sign() != 0 && d.MantExp(nil) > max(want.MantExp(nil), scale)-int(prec)+16 {
				t.Errorf("z = %v: %s off at 2^%d", z, name, d.MantExp(nil)-want.MantExp(nil))
			}
		}

		check("c0 = 1 − z·c2", c[0], new(big.Float).SetPrec(prec).Sub(newI(1), new(big.Float).SetPrec(prec).Mul(bz, c[2])))
		check("c1 = 1 − z·c3", c[1], new(big.Float).SetPrec(prec).Sub(newI(1), new(big.Float).SetPrec(prec).Mul(bz, c[3])))

		zc1 := new(big.Float).SetPrec(prec).Mul(c[1], c[1])
		zc1.Mul(zc1, bz)
		check("c0² + z·c1² = 1", new(big.Float).SetPrec(prec).Add(new(big.Float).SetPrec(prec).Mul(c[0], c[0]), zc1), newI(1))

		var want float64
		if z > 0 {
			want = math.Cos(math.Sqrt(z))
		} else {
			want = math.Cosh(math.Sqrt(-z))
		}

		if u := ulpsOf(want, c[0]); u > 4 {
			t.Errorf("z = %v: c0 is %.3g ulps from math's", z, u)
		}
	}
}

// TestOracleUniversalIsKepler: on an ellipse the universal state is the
// classical one, from the oracle's own eccentric anomaly at M = n·Δt — two
// formulations sharing nothing but the arithmetic. M is formed in float64,
// so the agreement is to its rounding, not to Prec.
func TestOracleUniversalIsKepler(t *testing.T) {
	for _, e := range []float64{0, 0.3, 0.9, 0.999} {
		q := 0.7
		a := q / (1 - e)
		n := math.Sqrt(gaussMu / (a * a * a))

		for _, dt := range []float64{1e-3, 3.7, 50, 180} {
			ux, uy, _, _ := keplerlab.UniversalStateExact(q, e, gaussMu, dt)
			kx, ky, _, _ := exactStateFromE(a, e, n, solveExact(n*dt, e))

			rel := absErr(mustF(ux), mustF(uy), kx, ky) / norm(kx, ky)
			if rel > 1e-13 {
				t.Errorf("e = %v, Δt = %v: universal and classical differ by %.3g relative", e, dt, rel)
			}
		}
	}
}

func mustF(b *big.Float) float64 {
	f, _ := b.Float64()

	return f
}

// TestUniversalAccuracy measures the universal path against the oracle: the
// path as it ships and the candidate, in ulps of |r| for the position and of
// |v| for the velocity, beside the floor the correctly rounded χ sets. The
// table is logged; nothing is asserted.
func TestUniversalAccuracy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping the universal sweep under -short: it is a measurement, not a gate")
	}

	type worst struct {
		v, e, dt float64
	}

	results := map[string]*worst{}

	note := func(key string, v, e, dt float64) {
		w := results[key]
		if w == nil {
			w = &worst{}
			results[key] = w
		}

		if v > w.v || math.IsNaN(v) {
			w.v, w.e, w.dt = v, e, dt
		}
	}

	es := []float64{0, 0.5, 0.9, 0.99, 0.999, 0.99999, 1 - 1e-8, 1, 1 + 1e-8, 1.00001, 1.001, 1.2, 2, 5}

	for _, e := range es {
		q := 1.0
		alpha := (1 - e) / q

		maxDt := 1e6
		if alpha > 0 {
			maxDt = math.Pi / math.Sqrt(gaussMu*alpha*alpha*alpha) * (1 - 1e-6)
		}

		for k := -48; k <= 48; k++ {
			dt := math.Pow(10, float64(k)/8)
			if dt > maxDt {
				continue
			}

			for _, sgn := range []float64{1, -1} {
				d := sgn * dt

				wx, wy, wvx, wvy := keplerlab.UniversalStateExact(q, e, gaussMu, d)
				wchi := keplerlab.UniversalExact(q, e, gaussMu, d)

				rn, vn := norm(wx, wy), norm(wvx, wvy)
				if alpha > 0 {
					// The classical contract's scale: below n·a = √(μ/a), the
					// speed at aphelion is set by the anomaly's own rounding.
					vn = math.Max(vn, math.Sqrt(gaussMu*alpha))
				}

				region := regime(e)

				// The contract's shape: χ to 2 ulps carried through
				// ∂state/∂χ — |v|·r/√μ for the position, √μ/r for the
				// velocity, since dχ/dt = √μ/r — plus 4 ulps of construction.
				{
					r := rn
					sqrtMu := math.Sqrt(gaussMu)
					absChi := math.Abs(mustF(wchi))
					bPos := 2*0x1p-52*absChi*norm(wvx, wvy)*r/sqrtMu + 4*0x1p-52*rn
					bVel := 2*0x1p-52*absChi*sqrtMu/r + 4*0x1p-52*vn

					for _, who := range []string{"production", "candidate"} {
						var x, y, vx, vy float64
						if who == "production" {
							_, x, y, vx, vy = prodState(q, e, gaussMu, d)
						} else {
							_, x, y, vx, vy = candState(q, e, gaussMu, d)
						}

						note(region+"/"+who+"/pos÷bound", absErr(x, y, wx, wy)/bPos, e, d)
						note(region+"/"+who+"/vel÷bound", absErr(vx, vy, wvx, wvy)/bVel, e, d)
					}
				}

				chiF := mustF(wchi)
				fx, fy, fvx, fvy := keplerlab.UniversalStateFromChi(q, e, gaussMu, newF(chiF))
				note(region+"/floor/pos", absErr(mustF(fx), mustF(fy), wx, wy)/rn/0x1p-52, e, d)
				note(region+"/floor/vel", absErr(mustF(fvx), mustF(fvy), wvx, wvy)/vn/0x1p-52, e, d)

				for _, who := range []string{"production", "stumpff-only", "candidate"} {
					var chi, x, y, vx, vy float64

					switch who {
					case "production":
						chi, x, y, vx, vy = prodState(q, e, gaussMu, d)
					case "stumpff-only":
						chi, x, y, vx, vy = mixState(q, e, gaussMu, d)
					default:
						chi, x, y, vx, vy = candState(q, e, gaussMu, d)
					}

					note(region+"/"+who+"/chi", ulpsOf(chi, wchi), e, d)
					note(region+"/"+who+"/pos", absErr(x, y, wx, wy)/rn/0x1p-52, e, d)
					note(region+"/"+who+"/vel", absErr(vx, vy, wvx, wvy)/vn/0x1p-52, e, d)
				}
			}
		}
	}

	keys := make([]string, 0, len(results))
	for k := range results {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	var b strings.Builder

	for _, k := range keys {
		unit := "ulps"
		if strings.HasSuffix(k, "bound") {
			unit = "of the bound"
		}

		w := results[k]
		fmt.Fprintf(&b, "%-50s %10.3g %-12s (e = %v, Δt = %.4g d)\n", k, w.v, unit, w.e, w.dt)
	}

	t.Log("\n" + b.String())

	worstAt := results["near-parabolic hyperbola/production/pos"]
	wx, wy, _, _ := keplerlab.UniversalStateExact(1, worstAt.e, gaussMu, worstAt.dt)
	t.Logf("r at the shipped path's worst point: %.4g au", norm(wx, wy))
}

// regime names the kind of orbit an eccentricity makes, as the tables group
// them.
func regime(e float64) string {
	switch {
	case e == 1:
		return "parabola"
	case e > 1 && e < 1.01:
		return "near-parabolic hyperbola"
	case e > 1:
		return "hyperbola"
	case e > 0.99:
		return "near-parabolic ellipse"
	default:
		return "ellipse"
	}
}

// TestStumpffAccuracy: the shipped Stumpff pair and the candidate's, against
// the oracle, worst ulps by band of z.
func TestStumpffAccuracy(t *testing.T) {
	type band struct {
		name   string
		lo, hi float64
	}

	bands := []band{
		{"0 < z < 0.1", 1e-12, 0.1},
		{"0.1 <= z < 1", 0.1, 1},
		{"1 <= z < 4", 1, 4},
		{"4 <= z <= π²", 4, math.Pi * math.Pi},
		{"−0.1 < z < 0", -0.1, -1e-12},
		{"−1 < z <= −0.1", -1, -0.1},
		{"−4 < z <= −1", -4, -1},
		{"−400 <= z <= −4", -400, -4},
	}

	var b strings.Builder

	for _, bd := range bands {
		var pc, ps, cc, cs, c0e, c1e float64

		for i := range 200 {
			var z float64
			if bd.lo > 0 {
				z = bd.lo * math.Pow(bd.hi/bd.lo, float64(i)/199)
			} else {
				z = -(-bd.hi) * math.Pow(bd.lo/bd.hi, float64(i)/199)
			}

			w := keplerlab.Stumpff(z)

			c, s := prodStumpff(z)
			pc, ps = math.Max(pc, ulpsOf(c, w[2])), math.Max(ps, ulpsOf(s, w[3]))

			c0, c1, c2, c3 := candStumpff(z)
			cc, cs = math.Max(cc, ulpsOf(c2, w[2])), math.Max(cs, ulpsOf(c3, w[3]))

			// c₀ = cos √z crosses zero at z = π²/4 and c₁ = sin √z/√z at
			// z = π², where a relative error is unbounded and meaningless:
			// both are judged in ulps of max(|c|, 1).
			c0e = math.Max(c0e, ulpsOfAtLeastOne(c0, w[0]))
			c1e = math.Max(c1e, ulpsOfAtLeastOne(c1, w[1]))
		}

		fmt.Fprintf(&b, "%-18s production C %6.3g  S %6.3g   candidate C %6.3g  S %6.3g  c0 %6.3g  c1 %6.3g ulps\n",
			bd.name, pc, ps, cc, cs, c0e, c1e)
	}

	t.Log("\n" + b.String())
}
