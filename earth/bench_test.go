package earth_test

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/earth"
)

func BenchmarkGeodeticToECEF(b *testing.B) {
	g, _ := earth.NewGeodetic(angle.Deg(10), angle.Deg(45), 100)
	ell := earth.WGS84()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = g.ToECEF(ell)
	}
}
