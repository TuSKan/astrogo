package coord_test

import (
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/atmosphere"
	"github.com/TuSKan/astrogo/coord"
	"github.com/TuSKan/astrogo/time"
	"github.com/TuSKan/astrogo/unit"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// The Context benchmarks in bench_test.go run with the Earth orientation the
// process holds, which is none unless a loader and a cache happen to be present.
// NewContext and SetTime are where it is read — DUT1 for the UT1 the rotation
// is built from, polar motion for the pole — so these pin it, absent and
// present, and leave the rest of those benchmarks as they were.

// benchEOPStart and benchEOPDays place a synthetic table over every epoch the
// coord benchmarks use: MJD 59900 for 1500 days.
const (
	benchEOPStart = 59900.0
	benchEOPDays  = 1500
)

// benchEOPModes are three states of Earth orientation. "untouched" is the
// library as a program that registers no loader has it, as this package's
// tests do: every lookup goes through the lazy load and finds nothing to load.
// It is not "zero", which RegisterModel makes skip the lazy load altogether,
// and until #953 the two differed by a global mutex on every lookup.
var benchEOPModes = []string{"untouched", "zero", "table"}

// pinEOP is the process-wide Earth orientation for the benchmark, restored to
// the library default when it ends. RegisterModel makes the choice
// authoritative, so a cache on disk can not replace it.
func pinEOP(b *testing.B, mode string) {
	b.Helper()

	want := "explicit"

	switch mode {
	case "untouched":
		time.ResetEOP()

		want = "zero"
	case "zero":
		time.RegisterModel(time.ZeroModel{})
	case "table":
		table, err := time.ParseFinals2000A(strings.NewReader(testutil.FinalsText(benchEOPStart, benchEOPDays)))
		if err != nil {
			b.Fatal(err)
		}

		time.RegisterModel(table)
	default:
		b.Fatalf("unknown EOP mode %q", mode)
	}

	b.Cleanup(time.ResetEOP)

	// After a lookup: one that found a bulletin to load would leave the source
	// "cache", and "untouched" would time a table.
	_ = time.FromJD(2461056.25, time.UTC).EOP()

	if got := time.EOPSource(); got != want {
		b.Fatalf("EOP source = %q after pinning %q, want %s: the benchmark would time ambient data", got, mode, want)
	}
}

// BenchmarkNewContextEOP is one Context built, with Earth orientation absent
// and present: the ~91 µs Apco13 solve and, in "table", the lookup feeding it.
func BenchmarkNewContextEOP(b *testing.B) {
	loc, err := coord.NewGeodetic(angle.Deg(2.1686), angle.Deg(41.3874), 0)
	if err != nil {
		b.Fatal(err)
	}

	for _, mode := range benchEOPModes {
		b.Run(mode, func(b *testing.B) {
			pinEOP(b, mode)

			t := time.FromJD(2461056.25, time.UTC)
			atm := atmosphere.StandardRefraction()

			b.ReportAllocs()

			for b.Loop() {
				_ = coord.NewContext(t, loc, atm)
			}
		})
	}
}

// BenchmarkSetTimeEOP is BenchmarkSetTime's two walks, run with Earth
// orientation absent and present: moving a Context reads it at every step.
func BenchmarkSetTimeEOP(b *testing.B) {
	loc, err := coord.NewGeodetic(angle.Deg(2.1686), angle.Deg(41.3874), 0)
	if err != nil {
		b.Fatal(err)
	}

	start := time.FromJD(2461056.25, time.UTC)

	for _, mode := range benchEOPModes {
		for _, c := range []struct {
			name  string
			steps int
		}{{"within the hour", 11}, {"a night", 96}} {
			b.Run(mode+"/"+c.name, func(b *testing.B) {
				pinEOP(b, mode)

				ctx := coord.NewContext(start, loc, atmosphere.StandardRefraction())

				steps := make([]time.Time, c.steps)
				for i := range steps {
					steps[i] = start.Add(unit.Minutes(5 * float64(i+1)))
				}

				b.ReportAllocs()

				for b.Loop() {
					ctx.SetTime(start)

					for _, t := range steps {
						ctx.SetTime(t)
					}
				}
			})
		}
	}
}

// BenchmarkSetTimeEOPParallel is a night's walk on every core at once, each
// goroutine moving its own copy of one Context, as a ranking's targets do with
// their urgency probes. The Context holds its bracket first, as RankObservables
// builds it, so each move interpolates and looks Earth orientation up. Per-op
// time is wall time across GOMAXPROCS goroutines, and falls as cores are added
// unless the moves contend; before #953 every lookup took shared locks.
func BenchmarkSetTimeEOPParallel(b *testing.B) {
	loc, err := coord.NewGeodetic(angle.Deg(2.1686), angle.Deg(41.3874), 0)
	if err != nil {
		b.Fatal(err)
	}

	start := time.FromJD(2461056.25, time.UTC)

	steps := make([]time.Time, 96)
	for i := range steps {
		steps[i] = start.Add(unit.Minutes(5 * float64(i+1)))
	}

	for _, mode := range benchEOPModes {
		b.Run(mode, func(b *testing.B) {
			pinEOP(b, mode)

			ctx := coord.NewContext(start, loc, atmosphere.StandardRefraction())
			ctx.SetTime(steps[len(steps)-1])
			ctx.SetTime(start)

			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					c := *ctx

					for _, t := range steps {
						c.SetTime(t)
					}
				}
			})
		})
	}
}
