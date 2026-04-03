package vector_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/vector"
)

var sink float64 // prevents dead-code elimination
var vecSink vector.Vec3

func BenchmarkDot(b *testing.B) {
	a := vector.V3(1, 2, 3)
	c := vector.V3(4, 5, 6)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = a.Dot(c)
	}
}

func BenchmarkCross(b *testing.B) {
	a := vector.V3(1, 2, 3)
	c := vector.V3(4, 5, 6)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vecSink = a.Cross(c)
	}
}

func BenchmarkNorm(b *testing.B) {
	v := vector.V3(1, 2, 3)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = v.Norm()
	}
}

func BenchmarkUnit(b *testing.B) {
	v := vector.V3(1, 2, 3)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vecSink = v.Unit()
	}
}

func BenchmarkFromSpherical(b *testing.B) {
	lon, lat := 1.234, 0.567
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vecSink = vector.FromSpherical(lon, lat)
	}
}

func BenchmarkToSpherical(b *testing.B) {
	v := vector.FromSpherical(1.234, 0.567)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink, sink = v.ToSpherical()
	}
}

func BenchmarkRotateZ(b *testing.B) {
	v := vector.V3(1, 0, 0)
	rad := math.Pi / 4
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vecSink = v.RotateZ(rad)
	}
}

func BenchmarkAdd(b *testing.B) {
	a := vector.V3(1, 2, 3)
	c := vector.V3(4, 5, 6)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vecSink = a.Add(c)
	}
}
