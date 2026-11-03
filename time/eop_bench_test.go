package time_test

import (
	"strings"
	"testing"

	"github.com/TuSKan/astrogo/time"

	"github.com/TuSKan/astrogo/internal/testutil"
)

// The benchmarks in bench_test.go convert scales with whatever Earth
// orientation the process happens to hold: none, unless a loader is registered
// and a cache is on disk, which one machine has and another does not. They can
// therefore not say what a UT1 costs with real data present, nor be compared
// across machines. These pin the model — explicitly zero, or a synthetic table
// covering the epochs benchmarked here — so each result is a property of the
// code.

const (
	// benchEOPStart is MJD 59900 (2022-10-04), and the table runs 1500 days,
	// to mid-2027: every epoch the time, coord and plan benchmarks use.
	benchEOPStart = 59900.0
	benchEOPDays  = 1500
)

// benchEOPModes are the two states a benchmark pins: "zero" is what the
// untouched library does, and "table" is Earth orientation present.
var benchEOPModes = []string{"zero", "table"}

// pinEOP makes the process-wide Earth orientation mode for the benchmark, and
// restores the library's default when it ends. RegisterModel marks the choice
// authoritative, so the lazy load can not replace it with a cache from disk.
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

// BenchmarkUTCToUT1EOP is one UT1 conversion with Earth orientation absent and
// present. The difference is the table lookup, the interpolation, and the DUT1
// applied to the epoch.
func BenchmarkUTCToUT1EOP(b *testing.B) {
	for _, mode := range benchEOPModes {
		b.Run(mode, func(b *testing.B) {
			pinEOP(b, mode)

			t := time.FromJD(2460000.5, time.UTC)

			if _, err := t.UT1(); err != nil {
				b.Fatal(err)
			}

			b.ReportAllocs()

			for b.Loop() {
				if _, err := t.UT1(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkEOPLookup is Time.EOP alone, the call behind every NewContext and
// SetTime. "table" must return orientation and "zero" must not, or the mode
// did not take.
func BenchmarkEOPLookup(b *testing.B) {
	for _, mode := range benchEOPModes {
		b.Run(mode, func(b *testing.B) {
			pinEOP(b, mode)

			t := time.FromJD(2460000.5, time.UTC)

			if got := t.EOP().DUT1 != 0; got != (mode == "table") {
				b.Fatalf("mode %q: EOP = %+v", mode, t.EOP())
			}

			b.ReportAllocs()

			for b.Loop() {
				_ = t.EOP()
			}
		})
	}
}

// BenchmarkEOPLookupParallel is the same lookup from every core at once, which
// is what a ranking or a batch transform does: each worker builds a Context,
// and each Context reads the model. Per-op time here is wall time across
// GOMAXPROCS goroutines, so it rises above BenchmarkEOPLookup's when the
// readers contend.
func BenchmarkEOPLookupParallel(b *testing.B) {
	for _, mode := range benchEOPModes {
		b.Run(mode, func(b *testing.B) {
			pinEOP(b, mode)

			t := time.FromJD(2460000.5, time.UTC)

			b.ReportAllocs()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_ = t.EOP()
				}
			})
		})
	}
}

// BenchmarkUTCToTAIParallel is the control for BenchmarkEOPLookupParallel: a
// conversion of similar cost that shares no state between goroutines. On a busy
// or oversubscribed machine every parallel benchmark reads slower than its
// serial twin, so a lookup is contended only if it falls behind this one, not
// merely behind its own serial time.
func BenchmarkUTCToTAIParallel(b *testing.B) {
	t := time.FromJD(2460000.5, time.UTC)

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = t.TAI()
		}
	})
}
