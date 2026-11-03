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

var benchEOPModes = []string{"zero", "table"}

// pinEOP is the process-wide Earth orientation for the benchmark, restored to
// the library default when it ends. RegisterModel makes the choice
// authoritative, so a cache on disk can not replace it.
func pinEOP(b *testing.B, mode string) {
	b.Helper()

	switch mode {
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

	if got := time.EOPSource(); got != "explicit" {
		b.Fatalf("EOP source = %q after pinning %q, want explicit: the benchmark would time ambient data", got, mode)
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
