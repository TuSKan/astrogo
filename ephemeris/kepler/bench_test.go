package kepler_test

import (
	"math"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/ephemeris/kepler"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"
)

var benchSink float64

// BenchmarkSolveKepler solves across the elliptic range: a main-belt
// eccentricity, a comet's, and one near parabolic, at mean anomalies spread
// over a revolution either side of perihelion.
func BenchmarkSolveKepler(b *testing.B) {
	es := []float64{0.08, 0.6, 0.967, 0.9999}

	ms := make([]float64, 64)
	for i := range ms {
		ms[i] = -math.Pi + (float64(i)+0.5)*2*math.Pi/float64(len(ms))
	}

	b.ReportAllocs()

	i := 0
	for b.Loop() {
		ea, err := kepler.SolveKepler(angle.Rad(ms[i%len(ms)]), es[i%len(es)])
		if err != nil {
			b.Fatal(err)
		}

		benchSink += ea.Radians()
		i++
	}
}

// BenchmarkStateAt is one heliocentric state of a Ceres-like orbit, through
// NewElements and the eccentric anomaly.
func BenchmarkStateAt(b *testing.B) {
	epoch := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.LocationUTC)

	el, err := kepler.NewElements(epoch, unit.AU(2.77), 0.0785, angle.Deg(10.6), angle.Deg(80.3), angle.Deg(73.6), angle.Deg(60))
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	i := 0
	for b.Loop() {
		pos, _, err := el.StateAt(epoch.Add(unit.Days(float64(i % 400))))
		if err != nil {
			b.Fatal(err)
		}

		benchSink += pos.X
		i++
	}
}
