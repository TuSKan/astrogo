package vector_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/vector"
)

//nolint:gochecknoglobals // benchmark sinks prevent dead-code elimination
var (
	sink    float64 // prevents dead-code elimination
	vecSink vector.Vec3
)

func BenchmarkDot(b *testing.B) {
	a := vector.V3(1, 2, 3)
	c := vector.V3(4, 5, 6)

	for b.Loop() {
		sink = a.Dot(c)
	}
}

func BenchmarkCross(b *testing.B) {
	a := vector.V3(1, 2, 3)
	c := vector.V3(4, 5, 6)

	for b.Loop() {
		vecSink = a.Cross(c)
	}
}

func BenchmarkNorm(b *testing.B) {
	v := vector.V3(1, 2, 3)

	for b.Loop() {
		sink = v.Norm()
	}
}

func BenchmarkUnit(b *testing.B) {
	v := vector.V3(1, 2, 3)

	for b.Loop() {
		vecSink = v.Unit()
	}
}

func BenchmarkFromSpherical(b *testing.B) {
	lon, lat := 1.234, 0.567

	for b.Loop() {
		vecSink = vector.FromSpherical(lon, lat)
	}
}

func BenchmarkToSpherical(b *testing.B) {
	v := vector.FromSpherical(1.234, 0.567)

	for b.Loop() {
		sink, sink = v.ToSpherical()
	}
}

func BenchmarkRotateZ(b *testing.B) {
	v := vector.V3(1, 0, 0)
	rad := math.Pi / 4

	for b.Loop() {
		vecSink = v.RotateZ(rad)
	}
}

func BenchmarkAdd(b *testing.B) {
	a := vector.V3(1, 2, 3)
	c := vector.V3(4, 5, 6)

	for b.Loop() {
		vecSink = a.Add(c)
	}
}
