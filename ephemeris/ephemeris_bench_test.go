package ephemeris_test

import (
	"testing"

	"github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

func BenchmarkStateSun(b *testing.B) {
	p := ephemeris.Default()
	tm := time.NowUTC()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = p.State(ephemeris.Sun, tm)
	}
}

func BenchmarkStateMoon(b *testing.B) {
	p := ephemeris.Default()
	tm := time.NowUTC()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = p.State(ephemeris.Moon, tm)
	}
}

func BenchmarkToICRS(b *testing.B) {
	pos := vector.Vec3{X: 1.0, Y: 0.5, Z: 0.2}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ephemeris.ToICRS(pos)
	}
}
