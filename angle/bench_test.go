package angle_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
)

// BenchmarkWrap2Pi measures the cost of normalizing to [0, 2π).
// Expected: ~5 ns/op (one math.Mod + one branch + no allocation).
func BenchmarkWrap2Pi(b *testing.B) {
	a := angle.Deg(540.123)

	b.ResetTimer()

	for range b.N {
		_ = a.Wrap2Pi()
	}
}

// BenchmarkWrapPi measures the cost of normalizing to (-π, π].
func BenchmarkWrapPi(b *testing.B) {
	a := angle.Deg(270.456)

	b.ResetTimer()

	for range b.N {
		_ = a.WrapPi()
	}
}

// BenchmarkSin measures the cost of a.Sin() vs raw math.Sin.
// Both should be within ~10% of each other since the wrapper is inlineable.
func BenchmarkSin(b *testing.B) {
	a := angle.Deg(37.5)

	b.ResetTimer()

	for range b.N {
		_ = a.Sin()
	}
}

// BenchmarkCos measures the cost of a.Cos().
func BenchmarkCos(b *testing.B) {
	a := angle.Deg(37.5)

	b.ResetTimer()

	for range b.N {
		_ = a.Cos()
	}
}

// BenchmarkDMSString measures formatting overhead.
func BenchmarkDMSString(b *testing.B) {
	a := angle.Deg(123.456789)

	b.ResetTimer()

	for range b.N {
		_ = a.DMSString(3)
	}
}

// BenchmarkHMSString measures HMS formatting overhead.
func BenchmarkHMSString(b *testing.B) {
	a := angle.Hour(12.3456789)

	b.ResetTimer()

	for range b.N {
		_ = a.HMSString(3)
	}
}

// BenchmarkParseDMS measures parsing overhead.
func BenchmarkParseDMS(b *testing.B) {
	s := `+12°34'56.789"`

	b.ResetTimer()

	for range b.N {
		_, _ = angle.ParseDMS(s)
	}
}

// BenchmarkDegConvert measures Deg constructor + Degrees accessor round-trip.
func BenchmarkDegConvert(b *testing.B) {
	v := math.Pi / 4

	b.ResetTimer()

	for range b.N {
		_ = angle.Rad(v).Degrees()
	}
}
