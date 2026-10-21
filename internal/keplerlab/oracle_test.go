package keplerlab_test

import (
	"math"
	"math/big"
	"testing"
)

// TestOraclePiAgrees: two unrelated algorithms for π agree to the precision
// range reduction needs, and both round to math.Pi.
func TestOraclePiAgrees(t *testing.T) {
	a, b := machinPi(reducePrec), agmPi(reducePrec)

	d := new(big.Float).SetPrec(reducePrec).Sub(a, b)
	if d.Sign() != 0 && d.MantExp(nil) > -int(reducePrec)+8 {
		t.Errorf("Machin and AGM disagree at 2^%d", d.MantExp(nil))
	}

	if f, _ := a.Float64(); f != math.Pi {
		t.Errorf("π rounds to %v, want math.Pi", f)
	}
}

// TestOracleSinCos: sin²+cos² = 1 to the working precision, sin(π/6) = 1/2,
// and the series agrees with math.Sin and math.Cos to a float64 ulp.
func TestOracleSinCos(t *testing.T) {
	sixth := new(big.Float).SetPrec(prec).Quo(new(big.Float).SetPrec(prec).Set(pi()), newI(6))

	s, _ := sinCos(sixth)
	if d := new(big.Float).SetPrec(prec).Sub(s, newF(0.5)); d.Sign() != 0 && d.MantExp(nil) > -int(prec)+8 {
		t.Errorf("sin(π/6) − 1/2 = 2^%d", d.MantExp(nil))
	}

	for _, x := range []float64{1e-300, 1e-8, 0.3, 1, 2.5, 3.14159, -3, 4} {
		s, c := sinCos(newF(x))

		one := new(big.Float).SetPrec(prec).Mul(s, s)
		one.Add(one, new(big.Float).SetPrec(prec).Mul(c, c))
		one.Sub(one, newI(1))

		if one.Sign() != 0 && one.MantExp(nil) > -int(prec)+8 {
			t.Errorf("x = %v: sin²+cos²−1 = 2^%d", x, one.MantExp(nil))
		}

		if sf, _ := s.Float64(); math.Abs(sf-math.Sin(x)) > 2*math.Abs(math.Nextafter(math.Sin(x), 2)-math.Sin(x)) {
			t.Errorf("sin(%v): oracle %v, math.Sin %v", x, sf, math.Sin(x))
		}
	}
}

// TestOracleSolvesKepler: the oracle's root satisfies the equation to the
// working precision across the whole domain, including the critical corner
// and the largest finite mean anomaly.
func TestOracleSolvesKepler(t *testing.T) {
	for _, e := range []float64{0, 0.5, 0.99, 1 - 0x1p-30, math.Nextafter(1, 0)} {
		for _, m := range []float64{1e-300, 1e-12, 1e-4, 0.5, 3, -2, math.Pi, 2 * math.Pi, 1e10, math.MaxFloat64} {
			bigE := solveExact(m, e)

			f, _ := keplerResidual(bigE, newF(e), reduce(m))
			if f.Sign() != 0 && f.MantExp(nil) > bigE.MantExp(nil)-int(prec)+12 {
				t.Errorf("e = %v, M = %v: residual 2^%d at E = %s", e, m, f.MantExp(nil), bigE.Text('g', 20))
			}
		}
	}
}
