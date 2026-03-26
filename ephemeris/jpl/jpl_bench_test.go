package jpl_test

import (
	"testing"

	"github.com/TuSKan/astrogo/ephemeris/jpl"
)

func BenchmarkEvalChebyshev(b *testing.B) {
	coeffs := make([]float64, 14) // Typical length for planets
	for i := range coeffs {
		coeffs[i] = 0.1 * float64(i)
	}
	tau := 0.5
	radius := 100.0

	b.Run("PositionOnly", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			jpl.EvalChebyshev(coeffs, tau, radius, false)
		}
	})

	b.Run("WithDerivative", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			jpl.EvalChebyshev(coeffs, tau, radius, true)
		}
	})
}
