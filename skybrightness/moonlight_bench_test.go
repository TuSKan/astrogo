package skybrightness

import (
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/coord"
)

// BenchmarkMoonlightRadiance benchmarks the full Moonlight component including
// the ephemeris lookup and alt-az transform.
func BenchmarkMoonlightRadiance(b *testing.B) {
	b.ReportAllocs()

	ctx := benchContext(b)
	m := NewMoonlight()
	aa := coord.NewAltAz(angle.Deg(45), angle.Deg(90))

	var sink Nanolambert

	for range b.N {
		r, err := m.Radiance(aa, ctx)
		if err != nil {
			b.Fatalf("Radiance: %v", err)
		}

		sink = r
	}

	_ = sink
}
