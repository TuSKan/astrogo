package ephemeris_test

import (
	"testing"

	eph "github.com/TuSKan/astrogo/ephemeris"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/vector"
)

// benchEpoch is fixed, not time.NowUTC: a benchmark whose result depends on the
// day it is run cannot be compared with the one before it.
func benchEpoch() time.Time {
	return time.Date(2026, time.March, 20, 22, 0, 0, 0, time.LocationUTC)
}

func BenchmarkStateSun(b *testing.B) {
	p := eph.Default()
	tm := benchEpoch()

	for b.Loop() {
		if _, err := eph.At(p, tm).Geometric(eph.Sun, eph.Earth); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStateMoon(b *testing.B) {
	p := eph.Default()
	tm := benchEpoch()

	for b.Loop() {
		if _, err := eph.At(p, tm).Geometric(eph.Moon, eph.Earth); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkToICRS(b *testing.B) {
	pos := vector.Vec3{X: 1.0, Y: 0.5, Z: 0.2}

	for b.Loop() {
		if _, err := eph.ToICRS(pos); err != nil {
			b.Fatal(err)
		}
	}
}
