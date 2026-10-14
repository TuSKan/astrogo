package vector_test

import (
	"math"
	"math/big"
	"testing"

	"github.com/TuSKan/astrogo/vector"
)

// exactColatitude returns atan(√(x²+y²)/|z|), the angle from the nearer pole
// of the float64 vector (x, y, z) exactly as given, in 300-bit arithmetic.
// It sums atan's Taylor series, which converges fast for the colatitudes
// below 1e-4 rad it is used on here.
func exactColatitude(x, y, z float64) float64 {
	const prec = 300

	f := func(v float64) *big.Float { return new(big.Float).SetPrec(prec).SetFloat64(v) }

	r2 := new(big.Float).SetPrec(prec).Mul(f(x), f(x))
	r2.Add(r2, new(big.Float).SetPrec(prec).Mul(f(y), f(y)))

	u := new(big.Float).SetPrec(prec).Sqrt(r2)
	u.Quo(u, f(math.Abs(z)))

	u2 := new(big.Float).SetPrec(prec).Mul(u, u)
	term := new(big.Float).SetPrec(prec).Set(u)
	sum := new(big.Float).SetPrec(prec)

	for k := range 30 {
		t := new(big.Float).SetPrec(prec).Quo(term, f(float64(2*k+1)))
		if k%2 == 0 {
			sum.Add(sum, t)
		} else {
			sum.Sub(sum, t)
		}

		term.Mul(term, u2)
	}

	c, _ := sum.Float64()

	return c
}

// TestToSphericalKeepsItsPrecisionAtThePoles holds the latitude to the exact
// colatitude of the vector it was given, close to both poles (#754).
//
// It was asin(Z/|v|), and asin's slope is unbounded as its argument nears 1:
// one ulp of Z/|v| there is about √(2·1.1e-16) ≈ 1.5e-8 rad of angle. The
// error measured 0.0034 mas at 2″ from a pole, 0.34 mas at 0.02″ and 2.7 mas
// at 0.002″, as large as the colatitude itself. atan2(Z, √(X²+Y²)), SOFA's
// iauC2s form, stays within an ulp of π/2 everywhere.
func TestToSphericalKeepsItsPrecisionAtThePoles(t *testing.T) {
	t.Parallel()

	const (
		mas   = math.Pi / 180 / 3600 / 1000
		bound = 1e-3 * mas // a microarcsecond
		steps = 97
	)

	for _, colat := range []float64{1e-4, 1e-5, 1e-6, 1e-7, 1e-8, 1e-9} {
		var worst float64

		for i := range steps {
			lon := 2 * math.Pi * (float64(i) + 0.37) / steps

			for _, south := range []bool{false, true} {
				v := vector.FromSpherical(lon, math.Pi/2-colat)
				if south {
					v.Z = -v.Z
				}

				_, lat := v.ToSpherical()

				got := math.Pi/2 - math.Abs(lat)
				if (lat < 0) != south {
					t.Fatalf("colatitude %.0e, lon %.4f, south %t: latitude %.17g has the wrong sign", colat, lon, south, lat)
				}

				worst = math.Max(worst, math.Abs(got-exactColatitude(v.X, v.Y, v.Z)))
			}
		}

		if worst > bound {
			t.Errorf("colatitude %.0e rad: latitude off by up to %.4g mas, want under %.0g mas", colat, worst/mas, bound/mas)
		}
	}
}

// TestToSphericalLongitudeStaysBelowTwoPi holds the longitude to its
// documented [0, 2π). A negative atan2 less than half an ulp of 2π from zero
// has no sum with 2π but 2π itself, so (1, -1e-20, 0) returned exactly 2π.
func TestToSphericalLongitudeStaysBelowTwoPi(t *testing.T) {
	t.Parallel()

	for _, y := range []float64{-1e-20, -1e-17, -4e-16, -math.SmallestNonzeroFloat64, -1e-15, -1e-3} {
		lon, _ := vector.V3(1, y, 0).ToSpherical()
		if lon < 0 || lon >= 2*math.Pi {
			t.Errorf("V3(1, %g, 0).ToSpherical() longitude = %.17g, want in [0, 2π)", y, lon)
		}
	}
}
