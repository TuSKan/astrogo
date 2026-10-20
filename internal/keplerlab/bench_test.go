//go:build validation

// The lab's benchmarks are behind the validation tag so that CI's Benchmarks
// job, which runs every untagged benchmark six times on each push to main,
// does not spend two minutes on comparisons docs/kepler.md has already made.
// The lab's tests stay untagged: they run on every platform, which is how
// the arm64 rounding in reduceExact was found. Run these with
//
//	go test -tags=validation -run='^$' -bench=. -count=6 ./internal/keplerlab/

package keplerlab_test

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/TuSKan/astrogo/unit"
)

// samples is a fixed set of (M, e) pairs: M uniform over a revolution, e
// from the given list, drawn from a fixed seed so every run times the same
// work.
func samples(es []float64, critical bool) (ms, ecc []float64) {
	r := rand.New(rand.NewPCG(7, 11))

	const n = 4096

	ms, ecc = make([]float64, n), make([]float64, n)

	for i := range n {
		ecc[i] = es[i%len(es)]

		if critical {
			ms[i] = 0.0045 * r.Float64() // Tommasini & Olivieri's critical band
		} else {
			ms[i] = math.Pi * (2*r.Float64() - 1)
		}
	}

	return ms, ecc
}

var sink float64

// BenchmarkSolvers times every candidate per solve: over the eccentricities
// of real orbits (planets to long-period comets), and in the critical region.
func BenchmarkSolvers(b *testing.B) {
	for _, set := range []struct {
		name     string
		es       []float64
		critical bool
	}{
		{"typical", []float64{0.0167, 0.0934, 0.2056, 0.25, 0.6, 0.967}, false},
		{"high-e", []float64{0.99, 0.9999, 0.999999}, false},
		{"critical", []float64{0.995, 0.9999, 0.999999, 1 - 1e-10}, true},
	} {
		ms, es := samples(set.es, set.critical)

		for _, s := range solvers {
			b.Run(set.name+"/"+s.name, func(b *testing.B) {
				for b.Loop() {
					for i := range ms {
						sink += s.solve(ms[i], es[i])
					}
				}

				b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*len(ms)), "ns/solve")
			})
		}
	}
}

// BenchmarkPreparedOrbit: what preparing an orbit saves per state.
func BenchmarkPreparedOrbit(b *testing.B) {
	el := ceresLike(b)
	o := prepare(el)
	at := el.Epoch().Add(unit.Days(123.456))

	b.Run("StateAt", func(b *testing.B) {
		for b.Loop() {
			p, _, err := el.StateAt(at)
			if err != nil {
				b.Fatal(err)
			}

			sink += p.X
		}
	})

	b.Run("prepared", func(b *testing.B) {
		for b.Loop() {
			p, _ := o.stateAt(at)
			sink += p.X
		}
	})
}
