package sky_test

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/sky"
)

func BenchmarkSeparation(b *testing.B) {
	c1 := coord.ICRS{RA: angle.Deg(10), Dec: angle.Deg(20)}
	c2 := coord.ICRS{RA: angle.Deg(15), Dec: angle.Deg(25)}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sky.Separation(c1, c2)
	}
}

func BenchmarkAirmass(b *testing.B) {
	alt := angle.Deg(30)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = sky.Airmass(alt)
	}
}
